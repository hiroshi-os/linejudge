package diffparse_test

import (
	"testing"

	"github.com/hiroshi-os/linejudge/internal/diffparse"
)

func TestParseNewFile(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
new file mode 100644
--- /dev/null
+++ b/foo.go
@@ -0,0 +1,3 @@
+package foo
+
+func X() {}
`
	files, err := diffparse.Parse(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path() != "foo.go" {
		t.Fatalf("files=%+v", files)
	}
	if len(files[0].Hunks) != 1 {
		t.Fatalf("hunks=%d", len(files[0].Hunks))
	}
	got := []int{}
	for _, ln := range files[0].Hunks[0].Lines {
		if ln.Kind == '+' {
			got = append(got, ln.NewNo)
		}
	}
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("new numbers %v", got)
	}
}

func TestParseReplaceHunk(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
 package a
-func Old() {}
+func New() {}
`
	files, err := diffparse.Parse(diff)
	if err != nil {
		t.Fatal(err)
	}
	h := files[0].Hunks[0]
	var plus, minus int
	for _, ln := range h.Lines {
		if ln.Kind == '+' {
			plus++
			if ln.NewNo != 2 {
				t.Fatalf("plus line %d", ln.NewNo)
			}
		}
		if ln.Kind == '-' {
			minus++
		}
	}
	if plus != 1 || minus != 1 {
		t.Fatalf("plus=%d minus=%d", plus, minus)
	}
}
