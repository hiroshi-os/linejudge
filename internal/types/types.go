package types

import "time"

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityError    Severity = "error"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

var SeverityRank = map[Severity]int{
	SeverityCritical: 4,
	SeverityError:    3,
	SeverityWarning:  2,
	SeverityInfo:     1,
}

type ReviewStatus string

const (
	StatusQueued    ReviewStatus = "queued"
	StatusRunning   ReviewStatus = "running"
	StatusCompleted ReviewStatus = "completed"
	StatusFailed    ReviewStatus = "failed"
)

type FindingSource string

const (
	SourceStatic FindingSource = "static"
	SourceLLM    FindingSource = "llm"
)

// Finding is the product's canonical review comment model.
// GitHub review-comment fields (path/line/side/body) are a projection of this.
type Finding struct {
	ID         string        `json:"id"`
	ReviewID   string        `json:"review_id"`
	CheckID    string        `json:"check_id"`
	Category   string        `json:"category"`
	Severity   Severity      `json:"severity"`
	Path       string        `json:"path"`
	Line       int           `json:"line"`
	EndLine    int           `json:"end_line,omitempty"`
	Side       string        `json:"side"`
	Title      string        `json:"title"`
	Body       string        `json:"body"`
	Rationale  string        `json:"rationale,omitempty"`
	Suggestion string        `json:"suggestion,omitempty"`
	Confidence float64       `json:"confidence"`
	Source     FindingSource `json:"source"`
	Fingerprint string       `json:"fingerprint"`
}

// ReviewComment is the GitHub pull-request review comment shape
// (Create Review Comment / Create a review API).
type ReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
}

type Repo struct {
	ID             string    `json:"id"`
	Owner          string    `json:"owner"`
	Name           string    `json:"name"`
	FullName       string    `json:"full_name"`
	InstallationID string    `json:"installation_id"`
	ConnectedAt    time.Time `json:"connected_at"`
}

type Review struct {
	ID           string       `json:"id"`
	RepoID       string       `json:"repo_id"`
	RepoFullName string       `json:"repo_full_name"`
	PRNumber     int          `json:"pr_number"`
	Title        string       `json:"title"`
	SHA          string       `json:"sha"`
	Base         string       `json:"base"`
	Head         string       `json:"head"`
	EventType    string       `json:"event_type"`
	DeliveryID   string       `json:"delivery_id"`
	Fixture      string       `json:"fixture,omitempty"`
	Status       ReviewStatus `json:"status"`
	Error        string       `json:"error,omitempty"`
	FindingCount int          `json:"finding_count"`
	CommentCount int          `json:"comment_count"`
	CreatedAt    time.Time    `json:"created_at"`
	StartedAt    *time.Time   `json:"started_at,omitempty"`
	FinishedAt   *time.Time   `json:"finished_at,omitempty"`
	Summary      string       `json:"summary,omitempty"`
}

type PipelineEvent struct {
	ID         int64     `json:"id"`
	ReviewID   string    `json:"review_id"`
	Stage      string    `json:"stage"`
	Status     string    `json:"status"`
	Detail     string    `json:"detail,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type StoredComment struct {
	ID              string    `json:"id"`
	ReviewID        string    `json:"review_id"`
	FindingID       string    `json:"finding_id"`
	Path            string    `json:"path"`
	Line            int       `json:"line"`
	Side            string    `json:"side"`
	Body            string    `json:"body"`
	GitHubCommentID string    `json:"github_comment_id,omitempty"`
	PublishedAt     time.Time `json:"published_at"`
}

type CheckConfig struct {
	Enabled   bool     `json:"enabled"`
	Severity  Severity `json:"severity,omitempty"`
	Threshold int      `json:"threshold,omitempty"`
	Approve   bool     `json:"approve,omitempty"`
}

type AppConfig struct {
	MinSeverity Severity               `json:"min_severity"`
	IgnorePaths []string               `json:"ignore_paths"`
	MaxComments int                    `json:"max_comments"`
	Checks      map[string]CheckConfig `json:"checks"`
	LLMProvider string                 `json:"llm_provider"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		MinSeverity: SeverityInfo,
		IgnorePaths: []string{"vendor/**", "node_modules/**", "**/*.min.js", "dist/**", "go.sum"},
		MaxComments: 25,
		LLMProvider: "mock",
		Checks: map[string]CheckConfig{
			"secrets":     {Enabled: true, Severity: SeverityCritical},
			"complexity":  {Enabled: true, Severity: SeverityWarning, Threshold: 12},
			"bugs":        {Enabled: true, Severity: SeverityError},
			"insecure":    {Enabled: true, Severity: SeverityError},
			"js-unsafe":   {Enabled: true, Severity: SeverityError},
			"llm-assist":  {Enabled: true, Approve: false},
		},
	}
}

type PackedLine struct {
	Kind  string `json:"kind"`
	OldNo int    `json:"old_no,omitempty"`
	NewNo int    `json:"new_no,omitempty"`
	Text  string `json:"text"`
}

type PackedHunk struct {
	Header   string       `json:"header"`
	OldStart int          `json:"old_start"`
	NewStart int          `json:"new_start"`
	Lines    []PackedLine `json:"lines"`
}

type PackedFile struct {
	Path     string       `json:"path"`
	Language string       `json:"language"`
	Added    int          `json:"added"`
	Deleted  int          `json:"deleted"`
	Hunks    []PackedHunk `json:"hunks"`
}

type LabeledIssue struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	LineEnd     int    `json:"line_end"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type EvalReport struct {
	GeneratedAt     time.Time          `json:"generated_at"`
	Fixtures        int                `json:"fixtures"`
	LabeledIssues   int                `json:"labeled_issues"`
	Findings        int                `json:"findings"`
	Hits            int                `json:"hits"`
	Precision       float64            `json:"precision"`
	Recall          float64            `json:"recall"`
	F1              float64            `json:"f1"`
	HitRate         float64            `json:"hit_rate"`
	PrecisionAt5    float64            `json:"precision_at_5"`
	PrecisionAt10   float64            `json:"precision_at_10"`
	CleanPRFalsePos int                `json:"clean_pr_false_positives"`
	ByFixture       []FixtureEval      `json:"by_fixture"`
	Notes           string             `json:"notes"`
}

type FixtureEval struct {
	Name            string   `json:"name"`
	Labeled         int      `json:"labeled"`
	Findings        int      `json:"findings"`
	Hits            int      `json:"hits"`
	Missed          []string `json:"missed"`
	FalsePositives  []string `json:"false_positives"`
	Precision       float64  `json:"precision"`
	Recall          float64  `json:"recall"`
	CleanPR         bool     `json:"clean_pr"`
}
