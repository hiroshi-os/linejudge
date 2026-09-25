package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hiroshi-os/linejudge/internal/types"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." && filepath.Dir(path) != "" {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS repos (
  id TEXT PRIMARY KEY,
  owner TEXT NOT NULL,
  name TEXT NOT NULL,
  full_name TEXT NOT NULL UNIQUE,
  installation_id TEXT NOT NULL DEFAULT '',
  connected_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS config (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS webhook_deliveries (
  delivery_id TEXT PRIMARY KEY,
  event TEXT NOT NULL,
  action TEXT,
  payload TEXT NOT NULL,
  received_at TEXT NOT NULL,
  result TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS reviews (
  id TEXT PRIMARY KEY,
  repo_id TEXT NOT NULL,
  pr_number INTEGER NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  sha TEXT NOT NULL,
  base_ref TEXT NOT NULL DEFAULT '',
  head_ref TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  delivery_id TEXT NOT NULL UNIQUE,
  fixture TEXT,
  status TEXT NOT NULL,
  error TEXT,
  finding_count INTEGER NOT NULL DEFAULT 0,
  comment_count INTEGER NOT NULL DEFAULT 0,
  summary TEXT,
  diff TEXT,
  packed_json TEXT,
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT,
  UNIQUE(repo_id, pr_number, sha, event_type)
);
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  review_id TEXT NOT NULL,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  run_after TEXT NOT NULL,
  locked_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS jobs_status ON jobs(status, run_after);
CREATE TABLE IF NOT EXISTS findings (
  id TEXT PRIMARY KEY,
  review_id TEXT NOT NULL,
  check_id TEXT NOT NULL,
  category TEXT NOT NULL,
  severity TEXT NOT NULL,
  path TEXT NOT NULL,
  line INTEGER NOT NULL,
  end_line INTEGER,
  side TEXT NOT NULL DEFAULT 'RIGHT',
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  rationale TEXT,
  suggestion TEXT,
  confidence REAL NOT NULL,
  source TEXT NOT NULL,
  fingerprint TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS findings_review ON findings(review_id);
CREATE TABLE IF NOT EXISTS comments (
  id TEXT PRIMARY KEY,
  review_id TEXT NOT NULL,
  finding_id TEXT NOT NULL,
  path TEXT NOT NULL,
  line INTEGER NOT NULL,
  side TEXT NOT NULL,
  body TEXT NOT NULL,
  github_comment_id TEXT,
  published_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pipeline_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  review_id TEXT NOT NULL,
  stage TEXT NOT NULL,
  status TEXT NOT NULL,
  detail TEXT,
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS eval_reports (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  report_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
`)
	return err
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	if t.IsZero() {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

func (s *Store) UpsertRepo(ctx context.Context, r types.Repo) error {
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO repos (id, owner, name, full_name, installation_id, connected_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(full_name) DO UPDATE SET installation_id=excluded.installation_id
`, r.ID, r.Owner, r.Name, r.FullName, r.InstallationID, r.ConnectedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListRepos(ctx context.Context) ([]types.Repo, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, owner, name, full_name, installation_id, connected_at FROM repos ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Repo
	for rows.Next() {
		var r types.Repo
		var ts string
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.FullName, &r.InstallationID, &ts); err != nil {
			return nil, err
		}
		r.ConnectedAt = parseTime(ts)
		out = append(out, r)
	}
	if out == nil {
		out = []types.Repo{}
	}
	return out, rows.Err()
}

func (s *Store) RepoByFullName(ctx context.Context, full string) (types.Repo, error) {
	var r types.Repo
	var ts string
	err := s.DB.QueryRowContext(ctx, `SELECT id, owner, name, full_name, installation_id, connected_at FROM repos WHERE full_name=?`, full).
		Scan(&r.ID, &r.Owner, &r.Name, &r.FullName, &r.InstallationID, &ts)
	if err != nil {
		return r, err
	}
	r.ConnectedAt = parseTime(ts)
	return r, nil
}

func (s *Store) GetConfig(ctx context.Context) (types.AppConfig, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT v FROM config WHERE k='app'`).Scan(&raw)
	if err == sql.ErrNoRows {
		cfg := types.DefaultConfig()
		if err := s.SaveConfig(ctx, cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err != nil {
		return types.AppConfig{}, err
	}
	var cfg types.AppConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return types.AppConfig{}, err
	}
	if cfg.Checks == nil {
		cfg.Checks = types.DefaultConfig().Checks
	}
	if cfg.MaxComments == 0 {
		cfg.MaxComments = 25
	}
	if cfg.LLMProvider == "" {
		cfg.LLMProvider = "mock"
	}
	return cfg, nil
}

func (s *Store) SaveConfig(ctx context.Context, cfg types.AppConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO config(k,v) VALUES('app', ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, string(b))
	return err
}

func (s *Store) RecordDelivery(ctx context.Context, deliveryID, event, action, payload, result string) (duplicate bool, err error) {
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO webhook_deliveries (delivery_id, event, action, payload, received_at, result)
VALUES (?, ?, ?, ?, ?, ?)`, deliveryID, event, action, payload, now(), result)
	if err != nil {
		if isUnique(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (s *Store) InsertReview(ctx context.Context, r types.Review, diff string) (created bool, err error) {
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO reviews (id, repo_id, pr_number, title, sha, base_ref, head_ref, event_type, delivery_id, fixture, status, diff, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.RepoID, r.PRNumber, r.Title, r.SHA, r.Base, r.Head, r.EventType, r.DeliveryID, nullStr(r.Fixture), r.Status, diff, now())
	if err != nil {
		if isUnique(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) Enqueue(ctx context.Context, jobID, reviewID string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO jobs (id, review_id, status, attempts, run_after, created_at) VALUES (?, ?, 'queued', 0, ?, ?)`,
		jobID, reviewID, now(), now())
	return err
}

func (s *Store) ClaimJob(ctx context.Context) (jobID, reviewID string, ok bool, err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.QueryRowContext(ctx, `
SELECT id, review_id FROM jobs
WHERE status='queued' AND run_after <= ?
ORDER BY created_at ASC LIMIT 1`, now()).Scan(&jobID, &reviewID)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE jobs SET status='running', locked_at=?, attempts=attempts+1 WHERE id=? AND status='queued'`, now(), jobID)
	if err != nil {
		return "", "", false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", "", false, nil
	}
	if err := tx.Commit(); err != nil {
		return "", "", false, err
	}
	return jobID, reviewID, true, nil
}

func (s *Store) FinishJob(ctx context.Context, jobID, status, lastErr string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jobs SET status=?, last_error=?, locked_at=NULL WHERE id=?`, status, nullStr(lastErr), jobID)
	return err
}

func (s *Store) RequeueJob(ctx context.Context, jobID string, after time.Duration, lastErr string) error {
	t := time.Now().UTC().Add(after).Format(time.RFC3339Nano)
	_, err := s.DB.ExecContext(ctx, `UPDATE jobs SET status='queued', last_error=?, run_after=?, locked_at=NULL WHERE id=?`, lastErr, t, jobID)
	return err
}

func (s *Store) JobAttempts(ctx context.Context, jobID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT attempts FROM jobs WHERE id=?`, jobID).Scan(&n)
	return n, err
}

func (s *Store) GetReview(ctx context.Context, id string) (types.Review, string, error) {
	return s.scanReview(ctx, `SELECT r.id, r.repo_id, COALESCE(repo.full_name,''), r.pr_number, r.title, r.sha, r.base_ref, r.head_ref, r.event_type, r.delivery_id, COALESCE(r.fixture,''), r.status, COALESCE(r.error,''), r.finding_count, r.comment_count, COALESCE(r.summary,''), r.diff, r.created_at, r.started_at, r.finished_at
FROM reviews r LEFT JOIN repos repo ON repo.id=r.repo_id WHERE r.id=?`, id)
}

func (s *Store) GetReviewDiff(ctx context.Context, id string) (string, error) {
	var d string
	err := s.DB.QueryRowContext(ctx, `SELECT diff FROM reviews WHERE id=?`, id).Scan(&d)
	return d, err
}

func (s *Store) SetPacked(ctx context.Context, id string, packed []types.PackedFile) error {
	b, err := json.Marshal(packed)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE reviews SET packed_json=? WHERE id=?`, string(b), id)
	return err
}

func (s *Store) GetPacked(ctx context.Context, id string) ([]types.PackedFile, error) {
	var raw sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT packed_json FROM reviews WHERE id=?`, id).Scan(&raw); err != nil {
		return nil, err
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var p []types.PackedFile
	if err := json.Unmarshal([]byte(raw.String), &p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) ListReviews(ctx context.Context, repoID, status string, limit int) ([]types.Review, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT r.id, r.repo_id, COALESCE(repo.full_name,''), r.pr_number, r.title, r.sha, r.base_ref, r.head_ref, r.event_type, r.delivery_id, COALESCE(r.fixture,''), r.status, COALESCE(r.error,''), r.finding_count, r.comment_count, COALESCE(r.summary,''), r.created_at, r.started_at, r.finished_at
FROM reviews r LEFT JOIN repos repo ON repo.id=r.repo_id WHERE 1=1`
	args := []any{}
	if repoID != "" {
		q += ` AND r.repo_id=?`
		args = append(args, repoID)
	}
	if status != "" {
		q += ` AND r.status=?`
		args = append(args, status)
	}
	q += ` ORDER BY r.created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Review
	for rows.Next() {
		r, err := scanReviewRow(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if out == nil {
		out = []types.Review{}
	}
	return out, rows.Err()
}

func (s *Store) MarkRunning(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE reviews SET status='running', started_at=? WHERE id=?`, now(), id)
	return err
}

func (s *Store) MarkDone(ctx context.Context, id string, status types.ReviewStatus, summary, errMsg string, findings, comments int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE reviews SET status=?, summary=?, error=?, finding_count=?, comment_count=?, finished_at=? WHERE id=?`,
		status, summary, nullStr(errMsg), findings, comments, now(), id)
	return err
}

func (s *Store) AddEvent(ctx context.Context, ev types.PipelineEvent) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO pipeline_events (review_id, stage, status, detail, started_at, finished_at) VALUES (?,?,?,?,?,?)`,
		ev.ReviewID, ev.Stage, ev.Status, ev.Detail, ev.StartedAt.UTC().Format(time.RFC3339Nano), ev.FinishedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListEvents(ctx context.Context, reviewID string) ([]types.PipelineEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, review_id, stage, status, COALESCE(detail,''), started_at, finished_at FROM pipeline_events WHERE review_id=? ORDER BY id`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.PipelineEvent
	for rows.Next() {
		var e types.PipelineEvent
		var a, b string
		if err := rows.Scan(&e.ID, &e.ReviewID, &e.Stage, &e.Status, &e.Detail, &a, &b); err != nil {
			return nil, err
		}
		e.StartedAt, e.FinishedAt = parseTime(a), parseTime(b)
		out = append(out, e)
	}
	if out == nil {
		out = []types.PipelineEvent{}
	}
	return out, rows.Err()
}

func (s *Store) ReplaceFindings(ctx context.Context, reviewID string, fs []types.Finding) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM findings WHERE review_id=?`, reviewID); err != nil {
		return err
	}
	for _, f := range fs {
		_, err := tx.ExecContext(ctx, `INSERT INTO findings (id, review_id, check_id, category, severity, path, line, end_line, side, title, body, rationale, suggestion, confidence, source, fingerprint)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			f.ID, reviewID, f.CheckID, f.Category, f.Severity, f.Path, f.Line, f.EndLine, f.Side, f.Title, f.Body, f.Rationale, f.Suggestion, f.Confidence, f.Source, f.Fingerprint)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListFindings(ctx context.Context, reviewID string) ([]types.Finding, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, review_id, check_id, category, severity, path, line, end_line, side, title, body, COALESCE(rationale,''), COALESCE(suggestion,''), confidence, source, fingerprint FROM findings WHERE review_id=? ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'error' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END, line`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Finding
	for rows.Next() {
		var f types.Finding
		var end sql.NullInt64
		if err := rows.Scan(&f.ID, &f.ReviewID, &f.CheckID, &f.Category, &f.Severity, &f.Path, &f.Line, &end, &f.Side, &f.Title, &f.Body, &f.Rationale, &f.Suggestion, &f.Confidence, &f.Source, &f.Fingerprint); err != nil {
			return nil, err
		}
		if end.Valid {
			f.EndLine = int(end.Int64)
		}
		out = append(out, f)
	}
	if out == nil {
		out = []types.Finding{}
	}
	return out, rows.Err()
}

func (s *Store) ReplaceComments(ctx context.Context, reviewID string, cs []types.StoredComment) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE review_id=?`, reviewID); err != nil {
		return err
	}
	for _, c := range cs {
		_, err := tx.ExecContext(ctx, `INSERT INTO comments (id, review_id, finding_id, path, line, side, body, github_comment_id, published_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			c.ID, reviewID, c.FindingID, c.Path, c.Line, c.Side, c.Body, c.GitHubCommentID, c.PublishedAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListComments(ctx context.Context, reviewID string) ([]types.StoredComment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, review_id, finding_id, path, line, side, body, COALESCE(github_comment_id,''), published_at FROM comments WHERE review_id=? ORDER BY path, line`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.StoredComment
	for rows.Next() {
		var c types.StoredComment
		var ts string
		if err := rows.Scan(&c.ID, &c.ReviewID, &c.FindingID, &c.Path, &c.Line, &c.Side, &c.Body, &c.GitHubCommentID, &ts); err != nil {
			return nil, err
		}
		c.PublishedAt = parseTime(ts)
		out = append(out, c)
	}
	if out == nil {
		out = []types.StoredComment{}
	}
	return out, rows.Err()
}

func (s *Store) SaveEval(ctx context.Context, r types.EvalReport) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO eval_reports (report_json, created_at) VALUES (?, ?)`, string(b), now())
	return err
}

func (s *Store) LatestEval(ctx context.Context) (*types.EvalReport, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT report_json FROM eval_reports ORDER BY id DESC LIMIT 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r types.EvalReport
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) Stats(ctx context.Context) (map[string]int, error) {
	out := map[string]int{}
	row := func(q, k string) {
		var n int
		_ = s.DB.QueryRowContext(ctx, q).Scan(&n)
		out[k] = n
	}
	row(`SELECT COUNT(*) FROM repos`, "repos")
	row(`SELECT COUNT(*) FROM reviews`, "reviews")
	row(`SELECT COUNT(*) FROM reviews WHERE status='completed'`, "completed")
	row(`SELECT COUNT(*) FROM reviews WHERE status='failed'`, "failed")
	row(`SELECT COUNT(*) FROM findings`, "findings")
	row(`SELECT COUNT(*) FROM comments`, "comments")
	row(`SELECT COUNT(*) FROM webhook_deliveries`, "deliveries")
	return out, nil
}

func (s *Store) scanReview(ctx context.Context, q string, arg any) (types.Review, string, error) {
	var r types.Review
	var fixture, errMsg, summary, diff, created string
	var started, finished sql.NullString
	err := s.DB.QueryRowContext(ctx, q, arg).Scan(
		&r.ID, &r.RepoID, &r.RepoFullName, &r.PRNumber, &r.Title, &r.SHA, &r.Base, &r.Head,
		&r.EventType, &r.DeliveryID, &fixture, &r.Status, &errMsg, &r.FindingCount, &r.CommentCount, &summary, &diff, &created, &started, &finished)
	if err != nil {
		return r, "", err
	}
	r.Fixture = fixture
	r.Error = errMsg
	r.Summary = summary
	r.CreatedAt = parseTime(created)
	r.StartedAt = parseTimePtr(started)
	r.FinishedAt = parseTimePtr(finished)
	return r, diff, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanReviewRow(rows rowScanner, withDiff bool) (types.Review, error) {
	var r types.Review
	var fixture, errMsg, summary, created string
	var started, finished sql.NullString
	err := rows.Scan(&r.ID, &r.RepoID, &r.RepoFullName, &r.PRNumber, &r.Title, &r.SHA, &r.Base, &r.Head,
		&r.EventType, &r.DeliveryID, &fixture, &r.Status, &errMsg, &r.FindingCount, &r.CommentCount, &summary, &created, &started, &finished)
	if err != nil {
		return r, err
	}
	r.Fixture = fixture
	r.Error = errMsg
	r.Summary = summary
	r.CreatedAt = parseTime(created)
	r.StartedAt = parseTimePtr(started)
	r.FinishedAt = parseTimePtr(finished)
	return r, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUnique(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE") || contains(msg, "unique")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}
