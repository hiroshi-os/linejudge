package diffparse

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hiroshi-os/linejudge/internal/types"
)

type FileDiff struct {
	OldPath  string
	NewPath  string
	IsNew    bool
	IsDelete bool
	Hunks    []Hunk
}

func (f FileDiff) Path() string {
	if f.NewPath != "" && f.NewPath != "/dev/null" {
		return f.NewPath
	}
	return f.OldPath
}

type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Header   string
	Section  string
	Lines    []Line
}

type Line struct {
	Kind  rune // ' ', '+', '-'
	OldNo int
	NewNo int
	Text  string
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

func Parse(unified string) ([]FileDiff, error) {
	sc := bufio.NewScanner(strings.NewReader(unified))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var files []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	oldNo, newNo := 0, 0

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
			hunk = nil
		}
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushHunk()
			if cur != nil {
				files = append(files, *cur)
			}
			a, b := parseGitPaths(line)
			cur = &FileDiff{OldPath: a, NewPath: b}
		case strings.HasPrefix(line, "new file mode"):
			if cur != nil {
				cur.IsNew = true
			}
		case strings.HasPrefix(line, "deleted file mode"):
			if cur != nil {
				cur.IsDelete = true
			}
		case strings.HasPrefix(line, "--- "):
			if cur != nil {
				cur.OldPath = stripPrefix(line[4:])
			}
		case strings.HasPrefix(line, "+++ "):
			if cur != nil {
				cur.NewPath = stripPrefix(line[4:])
			}
		case strings.HasPrefix(line, "@@ "):
			flushHunk()
			m := hunkRe.FindStringSubmatch(line)
			if m == nil {
				return files, fmt.Errorf("malformed hunk header: %s", line)
			}
			oldStart := atoi(m[1])
			oldCount := 1
			if m[2] != "" {
				oldCount = atoi(m[2])
			}
			newStart := atoi(m[3])
			newCount := 1
			if m[4] != "" {
				newCount = atoi(m[4])
			}
			hunk = &Hunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
				Header:   line,
				Section:  strings.TrimSpace(m[5]),
			}
			oldNo, newNo = oldStart, newStart
		default:
			if hunk == nil || cur == nil {
				continue
			}
			if line == "" {
				line = " "
			}
			kind := rune(line[0])
			if kind != ' ' && kind != '+' && kind != '-' && kind != '\\' {
				continue
			}
			text := ""
			if len(line) > 1 {
				text = line[1:]
			}
			if kind == '\\' {
				continue
			}
			dl := Line{Kind: kind, Text: text}
			switch kind {
			case ' ':
				dl.OldNo, dl.NewNo = oldNo, newNo
				oldNo++
				newNo++
			case '+':
				dl.NewNo = newNo
				newNo++
			case '-':
				dl.OldNo = oldNo
				oldNo++
			}
			hunk.Lines = append(hunk.Lines, dl)
		}
	}
	flushHunk()
	if cur != nil {
		files = append(files, *cur)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func Pack(files []FileDiff, budgetPerFile int) []types.PackedFile {
	if budgetPerFile <= 0 {
		budgetPerFile = 12_000
	}
	out := make([]types.PackedFile, 0, len(files))
	for _, f := range files {
		if f.IsDelete {
			continue
		}
		pf := types.PackedFile{
			Path:     f.Path(),
			Language: languageOf(f.Path()),
		}
		used := 0
		for _, h := range f.Hunks {
			ph := types.PackedHunk{
				Header:   h.Header,
				OldStart: h.OldStart,
				NewStart: h.NewStart,
			}
			for _, ln := range h.Lines {
				if ln.Kind == '+' {
					pf.Added++
				}
				if ln.Kind == '-' {
					pf.Deleted++
				}
				pl := types.PackedLine{Kind: string(ln.Kind), OldNo: ln.OldNo, NewNo: ln.NewNo, Text: ln.Text}
				cost := len(ln.Text) + 8
				if used+cost > budgetPerFile {
					break
				}
				ph.Lines = append(ph.Lines, pl)
				used += cost
			}
			if len(ph.Lines) > 0 {
				pf.Hunks = append(pf.Hunks, ph)
			}
			if used >= budgetPerFile {
				break
			}
		}
		out = append(out, pf)
	}
	return out
}

func ConcatNew(p types.PackedFile) string {
	var b strings.Builder
	for _, h := range p.Hunks {
		for _, ln := range h.Lines {
			if ln.Kind == "-" {
				continue
			}
			b.WriteString(ln.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func languageOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".yml", ".yaml":
		return "yaml"
	case ".json":
		return "json"
	case ".sql":
		return "sql"
	case ".rs":
		return "rust"
	default:
		return "text"
	}
}

func parseGitPaths(line string) (string, string) {
	rest := strings.TrimPrefix(line, "diff --git ")
	parts := strings.Split(rest, " ")
	if len(parts) < 2 {
		return "", ""
	}
	return strings.TrimPrefix(parts[0], "a/"), strings.TrimPrefix(parts[1], "b/")
}

func stripPrefix(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	if p == "/dev/null" {
		return p
	}
	return p
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
