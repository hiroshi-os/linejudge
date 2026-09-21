package githubx

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hiroshi-os/linejudge/internal/types"
)

type PullRequestEvent struct {
	Action string `json:"action"`
	Number int    `json:"number"`
	PR     struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Head   struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Fixture string `json:"_fixture"`
}

type CheckSuiteEvent struct {
	Action     string `json:"action"`
	CheckSuite struct {
		HeadSHA      string `json:"head_sha"`
		HeadBranch   string `json:"head_branch"`
		PullRequests []struct {
			Number int `json:"number"`
			Head   struct {
				SHA string `json:"sha"`
				Ref string `json:"ref"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		} `json:"pull_requests"`
	} `json:"check_suite"`
	Repository struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Fixture string `json:"_fixture"`
}

type Ingest struct {
	Event        string
	Action       string
	RepoFullName string
	Owner        string
	Repo         string
	PRNumber     int
	Title        string
	SHA          string
	Base         string
	Head         string
	Installation string
	Fixture      string
	ShouldReview bool
}

func ParseEvent(event string, body []byte) (Ingest, error) {
	var in Ingest
	in.Event = event
	switch event {
	case "pull_request":
		var e PullRequestEvent
		if err := json.Unmarshal(body, &e); err != nil {
			return in, err
		}
		in.Action = e.Action
		in.RepoFullName = e.Repository.FullName
		in.Owner = e.Repository.Owner.Login
		in.Repo = e.Repository.Name
		in.PRNumber = e.PR.Number
		if in.PRNumber == 0 {
			in.PRNumber = e.Number
		}
		in.Title = e.PR.Title
		in.SHA = e.PR.Head.SHA
		in.Head = e.PR.Head.Ref
		in.Base = e.PR.Base.Ref
		if e.Installation.ID != 0 {
			in.Installation = fmt.Sprintf("%d", e.Installation.ID)
		}
		in.Fixture = e.Fixture
		switch e.Action {
		case "opened", "reopened", "synchronize", "ready_for_review":
			in.ShouldReview = true
		}
	case "check_suite":
		var e CheckSuiteEvent
		if err := json.Unmarshal(body, &e); err != nil {
			return in, err
		}
		in.Action = e.Action
		in.RepoFullName = e.Repository.FullName
		in.Owner = e.Repository.Owner.Login
		in.Repo = e.Repository.Name
		in.SHA = e.CheckSuite.HeadSHA
		in.Head = e.CheckSuite.HeadBranch
		in.Fixture = e.Fixture
		if e.Installation.ID != 0 {
			in.Installation = fmt.Sprintf("%d", e.Installation.ID)
		}
		if len(e.CheckSuite.PullRequests) > 0 {
			pr := e.CheckSuite.PullRequests[0]
			in.PRNumber = pr.Number
			in.SHA = pr.Head.SHA
			in.Head = pr.Head.Ref
			in.Base = pr.Base.Ref
		}
		in.Title = "check_suite " + e.Action
		in.ShouldReview = e.Action == "completed" || e.Action == "requested" || e.Action == "rerequested"
	default:
		in.ShouldReview = false
	}
	return in, nil
}

func VerifySignature(secret string, body []byte, header string) error {
	if secret == "" {
		return nil
	}
	if !strings.HasPrefix(header, "sha256=") {
		return fmt.Errorf("missing sha256 signature")
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return fmt.Errorf("bad signature encoding")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	got := mac.Sum(nil)
	if !hmac.Equal(want, got) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

type Publisher interface {
	Publish(ctx context.Context, review types.Review, comments []types.ReviewComment, body string) ([]string, error)
}

// MemoryPublisher stores GitHub-shaped review comments in process (demo / tests).
type MemoryPublisher struct {
	mu     sync.Mutex
	Posted [][]types.ReviewComment
}

func (m *MemoryPublisher) Publish(_ context.Context, _ types.Review, comments []types.ReviewComment, _ string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]types.ReviewComment(nil), comments...)
	m.Posted = append(m.Posted, cp)
	ids := make([]string, len(comments))
	for i := range comments {
		ids[i] = fmt.Sprintf("mock-%d-%d", len(m.Posted), i+1)
	}
	return ids, nil
}

// APIPublisher posts a pull request review when GITHUB_TOKEN (or an
// installation token) is present. Without a token it records locally
// like MemoryPublisher — the demo path never requires GitHub.
type APIPublisher struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
	Fallback MemoryPublisher
}

func NewPublisher() Publisher {
	tok := os.Getenv("GITHUB_TOKEN")
	if tok == "" {
		tok = os.Getenv("GITHUB_APP_INSTALLATION_TOKEN")
	}
	return &APIPublisher{
		Token:   tok,
		BaseURL: getenv("GITHUB_API_URL", "https://api.github.com"),
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *APIPublisher) Publish(ctx context.Context, review types.Review, comments []types.ReviewComment, body string) ([]string, error) {
	if p.Token == "" || review.RepoFullName == "" || review.PRNumber == 0 {
		return p.Fallback.Publish(ctx, review, comments, body)
	}
	type ghComment struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Side string `json:"side"`
		Body string `json:"body"`
	}
	payload := map[string]any{
		"commit_id": review.SHA,
		"event":     "COMMENT",
		"body":      body,
		"comments":  []ghComment{},
	}
	gcs := make([]ghComment, 0, len(comments))
	for _, c := range comments {
		gcs = append(gcs, ghComment{Path: c.Path, Line: c.Line, Side: c.Side, Body: c.Body})
	}
	payload["comments"] = gcs
	raw, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews", strings.TrimRight(p.BaseURL, "/"), review.RepoFullName, review.PRNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github review: %s: %s", resp.Status, truncate(string(b), 400))
	}
	var parsed struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(b, &parsed)
	ids := make([]string, len(comments))
	for i := range comments {
		ids[i] = fmt.Sprintf("gh-review-%d#%d", parsed.ID, i)
	}
	return ids, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
