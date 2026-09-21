package evalx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hiroshi-os/linejudge/internal/fixtures"
	"github.com/hiroshi-os/linejudge/internal/llm"
	"github.com/hiroshi-os/linejudge/internal/pipeline"
	"github.com/hiroshi-os/linejudge/internal/types"
)

type Options struct {
	FixturesDir string
	Config      types.AppConfig
	ApproveLLM  bool
}

func Run(ctx context.Context, opt Options) (types.EvalReport, error) {
	cfg := opt.Config
	if cfg.Checks == nil {
		cfg = types.DefaultConfig()
	}
	if opt.ApproveLLM {
		c := cfg.Checks["llm-assist"]
		c.Enabled = true
		c.Approve = true
		cfg.Checks["llm-assist"] = c
	}
	fxt, err := fixtures.LoadAll(opt.FixturesDir)
	if err != nil {
		return types.EvalReport{}, err
	}
	runner := pipeline.Runner{LLM: llm.Resolve(cfg.LLMProvider)}
	rep := types.EvalReport{
		GeneratedAt: time.Now().UTC(),
		Fixtures:    len(fxt),
		Notes:       "Hit = same path and finding line within ±2 of a labeled issue. Precision@k uses severity then confidence rank. Clean PRs contribute false positives only. LLM stage uses the configured provider (mock unless OPENAI_API_KEY is set).",
	}

	var allHits, allLabeled, allFindings int
	var pAt5Hits, pAt5N, pAt10Hits, pAt10N int

	for _, fx := range fxt {
		review := types.Review{
			ID:           "eval_" + fx.Meta.Name,
			RepoFullName: fx.Meta.Repo,
			PRNumber:     fx.Meta.PR,
			Title:        fx.Meta.Title,
			SHA:          fx.Meta.SHA,
			EventType:    fx.Meta.Event,
			Fixture:      fx.Meta.Name,
		}
		res, err := runner.Run(ctx, review, fx.Diff, cfg)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", fx.Meta.Name, err)
		}
		hits, missed, fps := match(fx.Labels, res.Findings, fx.Meta.Clean)
		fe := types.FixtureEval{
			Name:           fx.Meta.Name,
			Labeled:        len(fx.Labels),
			Findings:       len(res.Findings),
			Hits:           hits,
			Missed:         missed,
			FalsePositives: fps,
			CleanPR:        fx.Meta.Clean,
			Precision:      ratio(hits, len(res.Findings)),
			Recall:         ratio(hits, len(fx.Labels)),
		}
		if fx.Meta.Clean {
			fe.Precision = ratio(0, len(res.Findings))
			fe.Recall = 1
			rep.CleanPRFalsePos += len(res.Findings)
		}
		rep.ByFixture = append(rep.ByFixture, fe)
		allHits += hits
		allLabeled += len(fx.Labels)
		allFindings += len(res.Findings)

		top5 := take(res.Findings, 5)
		top10 := take(res.Findings, 10)
		pAt5Hits += countHits(fx.Labels, top5, fx.Meta.Clean)
		pAt5N += len(top5)
		pAt10Hits += countHits(fx.Labels, top10, fx.Meta.Clean)
		pAt10N += len(top10)
	}
	rep.LabeledIssues = allLabeled
	rep.Findings = allFindings
	rep.Hits = allHits
	rep.Precision = ratio(allHits, allFindings)
	rep.Recall = ratio(allHits, allLabeled)
	rep.HitRate = rep.Recall
	rep.F1 = f1(rep.Precision, rep.Recall)
	rep.PrecisionAt5 = ratio(pAt5Hits, pAt5N)
	rep.PrecisionAt10 = ratio(pAt10Hits, pAt10N)
	return rep, nil
}

func match(labels []types.LabeledIssue, findings []types.Finding, clean bool) (hits int, missed, fps []string) {
	hitLabel := map[int]bool{}
	usedFinding := map[int]bool{}
	if !clean {
		for i, lab := range labels {
			for j, f := range findings {
				if usedFinding[j] {
					continue
				}
				if hit(lab, f) {
					hitLabel[i] = true
					usedFinding[j] = true
					break
				}
			}
		}
		hits = len(hitLabel)
		for i, lab := range labels {
			if !hitLabel[i] {
				missed = append(missed, fmt.Sprintf("%s %s:%d", lab.ID, lab.Path, lab.Line))
			}
		}
	}
	for j, f := range findings {
		if clean || !usedFinding[j] {
			fps = append(fps, fmt.Sprintf("%s %s:%d %s", f.CheckID, f.Path, f.Line, f.Title))
		}
	}
	if missed == nil {
		missed = []string{}
	}
	if fps == nil {
		fps = []string{}
	}
	return hits, missed, fps
}

func countHits(labels []types.LabeledIssue, findings []types.Finding, clean bool) int {
	if clean {
		return 0
	}
	h, _, _ := match(labels, findings, false)
	return h
}

func hit(lab types.LabeledIssue, f types.Finding) bool {
	if filepath.ToSlash(lab.Path) != filepath.ToSlash(f.Path) {
		return false
	}
	end := lab.LineEnd
	if end < lab.Line {
		end = lab.Line
	}
	return f.Line >= lab.Line-2 && f.Line <= end+2
}

func take(fs []types.Finding, k int) []types.Finding {
	if len(fs) < k {
		return fs
	}
	return fs[:k]
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func f1(p, r float64) float64 {
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

func Print(rep types.EvalReport) {
	fmt.Println("linejudge eval")
	fmt.Printf("  fixtures           %d\n", rep.Fixtures)
	fmt.Printf("  labeled issues     %d\n", rep.LabeledIssues)
	fmt.Printf("  findings emitted   %d\n", rep.Findings)
	fmt.Printf("  hits               %d\n", rep.Hits)
	fmt.Printf("  precision          %.3f\n", rep.Precision)
	fmt.Printf("  recall / hit-rate  %.3f\n", rep.Recall)
	fmt.Printf("  f1                 %.3f\n", rep.F1)
	fmt.Printf("  precision@5        %.3f\n", rep.PrecisionAt5)
	fmt.Printf("  precision@10       %.3f\n", rep.PrecisionAt10)
	fmt.Printf("  clean-PR false pos %d\n", rep.CleanPRFalsePos)
	fmt.Println()
	for _, fx := range rep.ByFixture {
		flag := ""
		if fx.CleanPR {
			flag = " (clean)"
		}
		fmt.Printf("  %-18s labeled=%d findings=%d hits=%d P=%.2f R=%.2f%s\n",
			fx.Name, fx.Labeled, fx.Findings, fx.Hits, fx.Precision, fx.Recall, flag)
		for _, m := range fx.Missed {
			fmt.Printf("      miss  %s\n", m)
		}
	}
	fmt.Printf("\nmeasured_at %s\n", rep.GeneratedAt.Format(time.RFC3339))
}

func WriteFile(path string, rep types.EvalReport) error {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
