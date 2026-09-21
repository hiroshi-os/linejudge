package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hiroshi-os/linejudge/internal/evalx"
	"github.com/hiroshi-os/linejudge/internal/fixtures"
	"github.com/hiroshi-os/linejudge/internal/githubx"
	"github.com/hiroshi-os/linejudge/internal/idgen"
	"github.com/hiroshi-os/linejudge/internal/store"
	"github.com/hiroshi-os/linejudge/internal/types"
)

type Server struct {
	Store       *store.Store
	FixturesDir string
	WebhookSecret string
	mux         *http.ServeMux
}

func New(st *store.Store, fixturesDir string) *Server {
	s := &Server{
		Store:         st,
		FixturesDir:   fixturesDir,
		WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /v1/meta", s.meta)
	s.mux.HandleFunc("GET /v1/stats", s.stats)
	s.mux.HandleFunc("GET /v1/repos", s.listRepos)
	s.mux.HandleFunc("POST /v1/repos", s.addRepo)
	s.mux.HandleFunc("GET /v1/config", s.getConfig)
	s.mux.HandleFunc("PUT /v1/config", s.putConfig)
	s.mux.HandleFunc("GET /v1/reviews", s.listReviews)
	s.mux.HandleFunc("GET /v1/reviews/{id}", s.getReview)
	s.mux.HandleFunc("GET /v1/reviews/{id}/diff", s.getDiff)
	s.mux.HandleFunc("GET /v1/reviews/{id}/findings", s.getFindings)
	s.mux.HandleFunc("GET /v1/reviews/{id}/comments", s.getComments)
	s.mux.HandleFunc("GET /v1/reviews/{id}/events", s.getEvents)
	s.mux.HandleFunc("GET /v1/reviews/{id}/packed", s.getPacked)
	s.mux.HandleFunc("POST /webhooks/github", s.webhook)
	s.mux.HandleFunc("POST /v1/demo/replay/{name}", s.replay)
	s.mux.HandleFunc("GET /v1/fixtures", s.listFixtures)
	s.mux.HandleFunc("POST /v1/eval/run", s.runEval)
	s.mux.HandleFunc("GET /v1/eval/latest", s.latestEval)
}

func (s *Server) Handler() http.Handler {
	return cors(s.mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitHub-Event, X-GitHub-Delivery, X-Hub-Signature-256, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "service": "linejudge"})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"name":        "linejudge",
		"role":        getenv("LINEJUDGE_ROLE", "all"),
		"llm":         getenv("LINEJUDGE_LLM_PROVIDER", "mock"),
		"openai_key":  os.Getenv("OPENAI_API_KEY") != "",
		"github_token": os.Getenv("GITHUB_TOKEN") != "" || os.Getenv("GITHUB_APP_INSTALLATION_TOKEN") != "",
		"webhook_secret": s.WebhookSecret != "",
		"fixtures":    s.FixturesDir,
	})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.Stats(r.Context())
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.Store.ListRepos(r.Context())
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, repos)
}

func (s *Server) addRepo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Owner, Name, InstallationID string
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpError(w, err)
		return
	}
	if body.Owner == "" || body.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "owner and name required"})
		return
	}
	repo := types.Repo{
		ID:             idgen.New("repo"),
		Owner:          body.Owner,
		Name:           body.Name,
		FullName:       body.Owner + "/" + body.Name,
		InstallationID: body.InstallationID,
		ConnectedAt:    time.Now().UTC(),
	}
	if err := s.Store.UpsertRepo(r.Context(), repo); err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 201, repo)
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Store.GetConfig(r.Context())
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, cfg)
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var cfg types.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		httpError(w, err)
		return
	}
	if cfg.Checks == nil {
		writeJSON(w, 400, map[string]string{"error": "checks required"})
		return
	}
	if err := s.Store.SaveConfig(r.Context(), cfg); err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, cfg)
}

func (s *Server) listReviews(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := s.Store.ListReviews(r.Context(), q.Get("repo_id"), q.Get("status"), atoi(q.Get("limit"), 50))
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rev, _, err := s.Store.GetReview(r.Context(), id)
	if err == sql.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		httpError(w, err)
		return
	}
	findings, _ := s.Store.ListFindings(r.Context(), id)
	comments, _ := s.Store.ListComments(r.Context(), id)
	events, _ := s.Store.ListEvents(r.Context(), id)
	packed, _ := s.Store.GetPacked(r.Context(), id)
	writeJSON(w, 200, map[string]any{
		"review":   rev,
		"findings": findings,
		"comments": comments,
		"events":   events,
		"packed":   packed,
	})
}

func (s *Server) getDiff(w http.ResponseWriter, r *http.Request) {
	d, err := s.Store.GetReviewDiff(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, d)
}

func (s *Server) getFindings(w http.ResponseWriter, r *http.Request) {
	fs, err := s.Store.ListFindings(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, fs)
}

func (s *Server) getComments(w http.ResponseWriter, r *http.Request) {
	cs, err := s.Store.ListComments(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, cs)
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	ev, err := s.Store.ListEvents(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, ev)
}

func (s *Server) getPacked(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetPacked(r.Context(), r.PathValue("id"))
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) listFixtures(w http.ResponseWriter, r *http.Request) {
	names, err := fixtures.List(s.FixturesDir)
	if err != nil {
		httpError(w, err)
		return
	}
	var out []map[string]any
	for _, n := range names {
		fx, err := fixtures.Load(s.FixturesDir, n)
		if err != nil {
			continue
		}
		out = append(out, map[string]any{
			"name":   fx.Meta.Name,
			"repo":   fx.Meta.Repo,
			"pr":     fx.Meta.PR,
			"title":  fx.Meta.Title,
			"clean":  fx.Meta.Clean,
			"labels": len(fx.Labels),
		})
	}
	writeJSON(w, 200, out)
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		httpError(w, err)
		return
	}
	if err := githubx.VerifySignature(s.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")); err != nil {
		writeJSON(w, 401, map[string]string{"error": err.Error()})
		return
	}
	event := r.Header.Get("X-GitHub-Event")
	delivery := r.Header.Get("X-GitHub-Delivery")
	if delivery == "" {
		delivery = "anon_" + randHex(8)
	}
	dup, err := s.Store.RecordDelivery(r.Context(), delivery, event, "", string(body), "received")
	if err != nil {
		httpError(w, err)
		return
	}
	if dup {
		writeJSON(w, 200, map[string]any{"ok": true, "idempotent": true, "delivery_id": delivery})
		return
	}
	ing, err := githubx.ParseEvent(event, body)
	if err != nil {
		httpError(w, err)
		return
	}
	if !ing.ShouldReview {
		writeJSON(w, 202, map[string]any{"ok": true, "ignored": true, "event": event, "action": ing.Action})
		return
	}
	rev, created, err := s.enqueueFromIngest(r.Context(), delivery, ing, "")
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "created": created, "review": rev})
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fx, err := fixtures.Load(s.FixturesDir, name)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": err.Error()})
		return
	}
	delivery := "fix_" + name + "_" + randHex(6)
	ing, err := githubx.ParseEvent(fx.Meta.Event, fx.Webhook)
	if err != nil {
		httpError(w, err)
		return
	}
	ing.Fixture = name
	ing.ShouldReview = true
	if ing.RepoFullName == "" {
		ing.RepoFullName = fx.Meta.Repo
		ing.Owner = fx.Meta.Owner
		ing.Repo = fx.Meta.RepoName
	}
	if ing.PRNumber == 0 {
		ing.PRNumber = fx.Meta.PR
	}
	if ing.Title == "" {
		ing.Title = fx.Meta.Title
	}
	ing.SHA = fmt.Sprintf("%040x", time.Now().UnixNano())[:40]
	rev, created, err := s.enqueueFromIngest(r.Context(), delivery, ing, fx.Diff)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "created": created, "review": rev, "fixture": name})
}

func (s *Server) enqueueFromIngest(ctx context.Context, delivery string, ing githubx.Ingest, diffOverride string) (types.Review, bool, error) {
	if ing.RepoFullName == "" {
		return types.Review{}, false, fmt.Errorf("missing repository")
	}
	repo, err := s.Store.RepoByFullName(ctx, ing.RepoFullName)
	if err == sql.ErrNoRows {
		repo = types.Repo{
			ID:             idgen.New("repo"),
			Owner:          ing.Owner,
			Name:           ing.Repo,
			FullName:       ing.RepoFullName,
			InstallationID: ing.Installation,
			ConnectedAt:    time.Now().UTC(),
		}
		if err := s.Store.UpsertRepo(ctx, repo); err != nil {
			return types.Review{}, false, err
		}
	} else if err != nil {
		return types.Review{}, false, err
	}

	diff := diffOverride
	if diff == "" && ing.Fixture != "" {
		fx, err := fixtures.Load(s.FixturesDir, ing.Fixture)
		if err == nil {
			diff = fx.Diff
		}
	}
	if diff == "" {
		return types.Review{}, false, fmt.Errorf("no diff available (set _fixture on the payload or POST /v1/demo/replay/{name})")
	}

	rev := types.Review{
		ID:           idgen.New("rvw"),
		RepoID:       repo.ID,
		RepoFullName: repo.FullName,
		PRNumber:     ing.PRNumber,
		Title:        ing.Title,
		SHA:          ing.SHA,
		Base:         ing.Base,
		Head:         ing.Head,
		EventType:    ing.Event,
		DeliveryID:   delivery,
		Fixture:      ing.Fixture,
		Status:       types.StatusQueued,
		CreatedAt:    time.Now().UTC(),
	}
	created, err := s.Store.InsertReview(ctx, rev, diff)
	if err != nil {
		return rev, false, err
	}
	if !created {
		return rev, false, nil
	}
	if err := s.Store.Enqueue(ctx, idgen.New("job"), rev.ID); err != nil {
		return rev, false, err
	}
	return rev, true, nil
}

func (s *Server) runEval(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Store.GetConfig(r.Context())
	if err != nil {
		httpError(w, err)
		return
	}
	approve := true
	if v := r.URL.Query().Get("approve_llm"); v == "0" || v == "false" {
		approve = false
	}
	rep, err := evalx.Run(r.Context(), evalx.Options{
		FixturesDir: s.FixturesDir,
		Config:      cfg,
		ApproveLLM:  approve,
	})
	if err != nil {
		httpError(w, err)
		return
	}
	_ = s.Store.SaveEval(r.Context(), rep)
	writeJSON(w, 200, rep)
}

func (s *Server) latestEval(w http.ResponseWriter, r *http.Request) {
	rep, err := s.Store.LatestEval(r.Context())
	if err != nil {
		httpError(w, err)
		return
	}
	if rep == nil {
		writeJSON(w, 404, map[string]string{"error": "no eval yet"})
		return
	}
	writeJSON(w, 200, rep)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func httpError(w http.ResponseWriter, err error) {
	log.Printf("api: %v", err)
	writeJSON(w, 500, map[string]string{"error": err.Error()})
}

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func SeedDemoRepos(ctx context.Context, st *store.Store) error {
	now := time.Now().UTC()
	repos := []types.Repo{
		{ID: idgen.New("repo"), Owner: "acme", Name: "payments", FullName: "acme/payments", InstallationID: "1", ConnectedAt: now},
		{ID: idgen.New("repo"), Owner: "acme", Name: "gateway", FullName: "acme/gateway", InstallationID: "1", ConnectedAt: now},
		{ID: idgen.New("repo"), Owner: "acme", Name: "web-app", FullName: "acme/web-app", InstallationID: "1", ConnectedAt: now},
	}
	for _, r := range repos {
		if err := st.UpsertRepo(ctx, r); err != nil {
			return err
		}
	}
	_, _ = st.GetConfig(ctx)
	return nil
}
