package pipeline

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hiroshi-os/linejudge/internal/checks"
	"github.com/hiroshi-os/linejudge/internal/diffparse"
	"github.com/hiroshi-os/linejudge/internal/idgen"
	"github.com/hiroshi-os/linejudge/internal/llm"
	"github.com/hiroshi-os/linejudge/internal/types"
)

type Publisher interface {
	Publish(ctx context.Context, review types.Review, comments []types.ReviewComment, body string) (ids []string, err error)
}

type Result struct {
	Packed   []types.PackedFile
	Findings []types.Finding
	Comments []types.StoredComment
	Events   []types.PipelineEvent
	Summary  string
}

type Runner struct {
	Checks    checks.Engine
	LLM       llm.Provider
	Publisher Publisher
}

type stageNote struct {
	status string
	detail string
}

func (r Runner) Run(ctx context.Context, review types.Review, unified string, cfg types.AppConfig) (Result, error) {
	var res Result
	stage := func(name string, fn func() (stageNote, error)) error {
		start := time.Now().UTC()
		note, err := fn()
		ev := types.PipelineEvent{
			ReviewID:   review.ID,
			Stage:      name,
			Status:     note.status,
			Detail:     note.detail,
			StartedAt:  start,
			FinishedAt: time.Now().UTC(),
		}
		if ev.Status == "" {
			ev.Status = "ok"
		}
		if err != nil {
			ev.Status = "error"
			if ev.Detail == "" {
				ev.Detail = err.Error()
			}
		}
		res.Events = append(res.Events, ev)
		return err
	}

	var files []diffparse.FileDiff
	if err := stage("parse-diff", func() (stageNote, error) {
		var err error
		files, err = diffparse.Parse(unified)
		if err != nil {
			return stageNote{}, err
		}
		return stageNote{detail: fmt.Sprintf("%d files", len(files))}, nil
	}); err != nil {
		return res, err
	}

	if err := stage("pack-context", func() (stageNote, error) {
		res.Packed = diffparse.Pack(files, 12_000)
		nHunks := 0
		for _, p := range res.Packed {
			nHunks += len(p.Hunks)
		}
		return stageNote{detail: fmt.Sprintf("%d files, %d hunks", len(res.Packed), nHunks)}, nil
	}); err != nil {
		return res, err
	}

	var static []types.Finding
	if err := stage("static-checks", func() (stageNote, error) {
		static = r.Checks.Run(res.Packed, cfg)
		return stageNote{detail: fmt.Sprintf("%d findings", len(static))}, nil
	}); err != nil {
		return res, err
	}

	var assisted []types.Finding
	llmCfg := cfg.Checks["llm-assist"]
	if err := stage("llm-assist", func() (stageNote, error) {
		if !llmCfg.Enabled {
			return stageNote{status: "skipped", detail: "check disabled"}, nil
		}
		if !llmCfg.Approve {
			return stageNote{status: "skipped", detail: "not approved in config (fail-closed)"}, nil
		}
		prov := r.LLM
		if prov == nil {
			prov = llm.Resolve(cfg.LLMProvider)
		}
		out, err := prov.Review(ctx, res.Packed)
		if err != nil {
			return stageNote{status: "degraded", detail: err.Error()}, nil
		}
		assisted = out
		return stageNote{detail: fmt.Sprintf("provider=%s findings=%d", prov.Name(), len(assisted))}, nil
	}); err != nil {
		return res, err
	}

	if err := stage("merge-rank", func() (stageNote, error) {
		all := append(static, assisted...)
		res.Findings = mergeRank(review.ID, all, cfg)
		return stageNote{detail: fmt.Sprintf("%d after merge/policy (from %d)", len(res.Findings), len(all))}, nil
	}); err != nil {
		return res, err
	}

	if err := stage("publish", func() (stageNote, error) {
		comments := make([]types.ReviewComment, 0, len(res.Findings))
		for _, f := range res.Findings {
			comments = append(comments, types.ReviewComment{
				Path: f.Path,
				Line: f.Line,
				Side: "RIGHT",
				Body: f.Body,
			})
		}
		body := summaryMarkdown(review, res.Findings)
		res.Summary = body
		var ids []string
		var err error
		if r.Publisher != nil {
			ids, err = r.Publisher.Publish(ctx, review, comments, body)
			if err != nil {
				return stageNote{}, err
			}
		}
		now := time.Now().UTC()
		for i, f := range res.Findings {
			gh := ""
			if i < len(ids) {
				gh = ids[i]
			}
			res.Comments = append(res.Comments, types.StoredComment{
				ID:              idgen.New("cmt"),
				ReviewID:        review.ID,
				FindingID:       f.ID,
				Path:            f.Path,
				Line:            f.Line,
				Side:            "RIGHT",
				Body:            f.Body,
				GitHubCommentID: gh,
				PublishedAt:     now,
			})
		}
		return stageNote{detail: fmt.Sprintf("%d comments", len(res.Comments))}, nil
	}); err != nil {
		return res, err
	}

	return res, nil
}

func mergeRank(reviewID string, in []types.Finding, cfg types.AppConfig) []types.Finding {
	min := types.SeverityRank[cfg.MinSeverity]
	if min == 0 {
		min = 1
	}
	seen := map[string]bool{}
	var out []types.Finding
	for _, f := range in {
		if types.SeverityRank[f.Severity] < min {
			continue
		}
		if f.Side == "" {
			f.Side = "RIGHT"
		}
		f.ReviewID = reviewID
		key := f.Fingerprint
		if key == "" {
			key = fmt.Sprintf("%s:%s:%d:%s", f.CheckID, f.Path, f.Line, f.Title)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := types.SeverityRank[out[i].Severity], types.SeverityRank[out[j].Severity]
		if ri != rj {
			return ri > rj
		}
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	if cfg.MaxComments > 0 && len(out) > cfg.MaxComments {
		out = out[:cfg.MaxComments]
	}
	return out
}

func summaryMarkdown(r types.Review, fs []types.Finding) string {
	var crit, errn, warn, info int
	for _, f := range fs {
		switch f.Severity {
		case types.SeverityCritical:
			crit++
		case types.SeverityError:
			errn++
		case types.SeverityWarning:
			warn++
		default:
			info++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### linejudge review\n\n")
	fmt.Fprintf(&b, "PR **#%d** `%s` · %d findings (critical %d · error %d · warning %d · info %d)\n\n",
		r.PRNumber, shortSHA(r.SHA), len(fs), crit, errn, warn, info)
	b.WriteString("Comments are line-anchored. Static checks always run; LLM assist is a gated stage and is skipped unless approved in config.\n")
	return b.String()
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
