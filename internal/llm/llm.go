package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hiroshi-os/linejudge/internal/diffparse"
	"github.com/hiroshi-os/linejudge/internal/idgen"
	"github.com/hiroshi-os/linejudge/internal/types"
)

type Provider interface {
	Name() string
	Review(ctx context.Context, packed []types.PackedFile) ([]types.Finding, error)
}

func Resolve(name string) Provider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "openai":
		if os.Getenv("OPENAI_API_KEY") == "" {
			return Mock{}
		}
		return OpenAI{}
	default:
		return Mock{}
	}
}

// Mock is a deterministic stand-in so demo + eval work without keys.
// It is a structured rule layer over packed hunks — not a chat wrapper.
type Mock struct{}

func (Mock) Name() string { return "mock" }

type mockRule struct {
	id, category, title, rationale, suggestion string
	severity                                   types.Severity
	match                                      func(file types.PackedFile, text string, line types.PackedLine) bool
}

func (Mock) Review(_ context.Context, packed []types.PackedFile) ([]types.Finding, error) {
	rules := []mockRule{
		{
			id: "authz-gap", category: "authz", severity: types.SeverityError,
			title: "Handler looks reachable without an authz check",
			rationale: "The added handler talks to storage but the hunk has no caller-principal check. Unauthenticated reads/writes are a class of incident, not a style nit.",
			suggestion: "Require the session/installation at the mux and enforce tenant scope on the query.",
			match: func(file types.PackedFile, text string, line types.PackedLine) bool {
				if line.Kind != "+" {
					return false
				}
				if !regexp.MustCompile(`HandleFunc\(|http\.HandlerFunc|export (async )?function (GET|POST|PUT|DELETE)`).MatchString(line.Text) {
					return false
				}
				return !regexp.MustCompile(`(?i)(auth|RequireUser|middleware|session|principal)`).MatchString(text)
			},
		},
		{
			id: "n-plus-one", category: "perf", severity: types.SeverityWarning,
			title: "Query inside a loop (N+1)",
			rationale: "A DB/HTTP call in a per-item loop turns O(n) CPU into O(n) round trips.",
			suggestion: "Batch the lookup (WHERE id IN (...)) or join once.",
			match: func(_ types.PackedFile, text string, line types.PackedLine) bool {
				if line.Kind != "+" {
					return false
				}
				if !regexp.MustCompile(`Query(Row)?(Context)?\(|fetch\(|axios\.|\.Find\(`).MatchString(line.Text) {
					return false
				}
				return regexp.MustCompile(`\bfor\b|\.map\(|\.forEach\(`).MatchString(text)
			},
		},
		{
			id: "race-map", category: "concurrency", severity: types.SeverityError,
			title: "Shared map written from multiple goroutines",
			rationale: "Concurrent map write is undefined in Go and fails loudly in production, not in unit tests.",
			suggestion: "Guard with a mutex, or use a single owner goroutine / sync.Map with a documented discipline.",
			match: func(_ types.PackedFile, text string, line types.PackedLine) bool {
				if line.Kind != "+" {
					return false
				}
				if !strings.Contains(text, "go func") {
					return false
				}
				return regexp.MustCompile(`\w+\[[^\]]+\]\s*=`).MatchString(line.Text) &&
					!strings.Contains(text, "Mutex") && !strings.Contains(text, "RWMutex")
			},
		},
		{
			id: "ctx-ignored", category: "correctness", severity: types.SeverityWarning,
			title: "Request context dropped",
			rationale: "Spawning work without the request context means cancellation and deadlines do not propagate.",
			suggestion: "Pass ctx (or a derived context with a timeout) into the goroutine / client call.",
			match: func(_ types.PackedFile, _ string, line types.PackedLine) bool {
				if line.Kind != "+" {
					return false
				}
				return regexp.MustCompile(`go func\(\)\s*\{|context\.Background\(\)`).MatchString(line.Text)
			},
		},
		{
			id: "ssrf", category: "ssrf", severity: types.SeverityError,
			title: "Outbound request uses caller-controlled URL",
			rationale: "Fetching a URL from request input is SSRF unless the host is allowlisted.",
			suggestion: "Parse the URL, allowlist hosts, and block link-local / metadata ranges.",
			match: func(_ types.PackedFile, text string, line types.PackedLine) bool {
				if line.Kind != "+" {
					return false
				}
				if !regexp.MustCompile(`http\.(Get|Post|Head)\(|client\.Get\(|fetch\(`).MatchString(line.Text) {
					return false
				}
				return regexp.MustCompile(`r\.URL|req\.(Body|Form)|query\.|params\.|body\.url`).MatchString(text)
			},
		},
	}

	var out []types.Finding
	seen := map[string]bool{}
	for _, f := range packed {
		text := diffparse.ConcatNew(f)
		for _, h := range f.Hunks {
			for _, ln := range h.Lines {
				for _, r := range rules {
					if !r.match(f, text, ln) {
						continue
					}
					fp := fmt.Sprintf("llm-mock:%s:%s:%d", r.id, f.Path, ln.NewNo)
					if seen[fp] {
						continue
					}
					seen[fp] = true
					out = append(out, types.Finding{
						ID:          idgen.New("fnd"),
						CheckID:     "llm-assist",
						Category:    r.category,
						Severity:    r.severity,
						Path:        f.Path,
						Line:        ln.NewNo,
						EndLine:     ln.NewNo,
						Side:        "RIGHT",
						Title:       r.title,
						Body:        fmt.Sprintf("**%s** · `llm-assist` · %s\n\n%s\n\n**Fix:** %s\n\n<sub>linejudge llm-assist · provider mock</sub>", r.title, strings.ToUpper(string(r.severity)), r.rationale, r.suggestion),
						Rationale:   r.rationale,
						Suggestion:  r.suggestion,
						Confidence:  0.64,
						Source:      types.SourceLLM,
						Fingerprint: fp,
					})
				}
			}
		}
	}
	return out, nil
}

type OpenAI struct{}

func (OpenAI) Name() string { return "openai" }

func (OpenAI) Review(ctx context.Context, packed []types.PackedFile) ([]types.Finding, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return Mock{}.Review(ctx, packed)
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	payload, _ := json.Marshal(packed)
	if len(payload) > 80_000 {
		payload = payload[:80_000]
	}
	body := map[string]any{
		"model": model,
		"temperature": 0,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{`role`: "system", `content`: `You are one stage in a deterministic code-review pipeline, not a chatbot.
Return JSON {"findings":[{"path":"","line":1,"category":"","severity":"error|warning|info|critical","title":"","rationale":"","suggestion":"","confidence":0.0}]}
Only report issues with a concrete line in the packed RIGHT-side hunks. Prefer security, correctness, concurrency, authz. Skip nits and style. Max 8 findings.`},
			{"role": "user", "content": "Packed hunks:\n" + string(payload)},
		},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai: %s: %s", resp.Status, truncate(string(b), 400))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openai: empty choices")
	}
	var envelope struct {
		Findings []struct {
			Path, Category, Severity, Title, Rationale, Suggestion string
			Line                                                   int
			Confidence                                             float64
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(parsed.Choices[0].Message.Content), &envelope); err != nil {
		return nil, err
	}
	var out []types.Finding
	for _, f := range envelope.Findings {
		if f.Path == "" || f.Line <= 0 || f.Title == "" {
			continue
		}
		sev := types.Severity(strings.ToLower(f.Severity))
		if types.SeverityRank[sev] == 0 {
			sev = types.SeverityWarning
		}
		out = append(out, types.Finding{
			ID:          idgen.New("fnd"),
			CheckID:     "llm-assist",
			Category:    f.Category,
			Severity:    sev,
			Path:        f.Path,
			Line:        f.Line,
			EndLine:     f.Line,
			Side:        "RIGHT",
			Title:       f.Title,
			Body:        fmt.Sprintf("**%s** · `llm-assist` · %s\n\n%s\n\n**Fix:** %s\n\n<sub>linejudge llm-assist · provider openai</sub>", f.Title, strings.ToUpper(string(sev)), f.Rationale, f.Suggestion),
			Rationale:   f.Rationale,
			Suggestion:  f.Suggestion,
			Confidence:  f.Confidence,
			Source:      types.SourceLLM,
			Fingerprint: fmt.Sprintf("llm-openai:%s:%s:%d", f.Category, f.Path, f.Line),
		})
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
