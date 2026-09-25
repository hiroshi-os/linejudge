package checks_test

import (
	"strings"
	"testing"

	"github.com/hiroshi-os/linejudge/internal/checks"
	"github.com/hiroshi-os/linejudge/internal/diffparse"
	"github.com/hiroshi-os/linejudge/internal/types"
)

func TestSecretsAWS(t *testing.T) {
	diff := `diff --git a/config/prod.yaml b/config/prod.yaml
new file mode 100644
--- /dev/null
+++ b/config/prod.yaml
@@ -0,0 +1,2 @@
+region: us-east-1
+access_key_id: AKIAIOSFODNN7EXAMPLE
`
	files, err := diffparse.Parse(diff)
	if err != nil {
		t.Fatal(err)
	}
	packed := diffparse.Pack(files, 4000)
	fs := (checks.Engine{}).Run(packed, types.DefaultConfig())
	found := false
	for _, f := range fs {
		if f.CheckID == "secrets" && strings.Contains(f.Title, "AWS") && f.Line == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected AWS secret finding, got %#v", fs)
	}
}
