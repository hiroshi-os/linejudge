package evalx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiroshi-os/linejudge/internal/evalx"
	"github.com/hiroshi-os/linejudge/internal/types"
)

func fixturesDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(wd, "fixtures")
		if st, err := os.Stat(filepath.Join(p, "secret-leak", "diff.patch")); err == nil && !st.IsDir() {
			return p
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("fixtures/ not found")
	return ""
}

func TestHarnessMeasuresFixtures(t *testing.T) {
	rep, err := evalx.Run(context.Background(), evalx.Options{
		FixturesDir: fixturesDir(t),
		Config:      types.DefaultConfig(),
		ApproveLLM:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Fixtures < 5 {
		t.Fatalf("fixtures=%d", rep.Fixtures)
	}
	if rep.LabeledIssues < 10 {
		t.Fatalf("labeled=%d", rep.LabeledIssues)
	}
	if rep.Recall < 0.7 {
		t.Fatalf("recall too low: %.3f hits=%d labeled=%d by=%+v", rep.Recall, rep.Hits, rep.LabeledIssues, rep.ByFixture)
	}
	if rep.CleanPRFalsePos > 3 {
		t.Fatalf("clean PR FPs=%d", rep.CleanPRFalsePos)
	}
}
