package checks

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/hiroshi-os/linejudge/internal/idgen"
	"github.com/hiroshi-os/linejudge/internal/types"
)

type Engine struct{}

func (Engine) Run(packed []types.PackedFile, cfg types.AppConfig) []types.Finding {
	var out []types.Finding
	for _, f := range packed {
		if ignored(f.Path, cfg.IgnorePaths) {
			continue
		}
		if enabled(cfg, "secrets") {
			out = append(out, secrets(f, cfg)...)
		}
		if enabled(cfg, "complexity") {
			out = append(out, complexity(f, cfg)...)
		}
		if enabled(cfg, "bugs") {
			out = append(out, bugs(f, cfg)...)
		}
		if enabled(cfg, "insecure") {
			out = append(out, insecure(f, cfg)...)
		}
		if enabled(cfg, "js-unsafe") && (f.Language == "javascript" || f.Language == "typescript") {
			out = append(out, jsUnsafe(f, cfg)...)
		}
	}
	return out
}

func enabled(cfg types.AppConfig, id string) bool {
	c, ok := cfg.Checks[id]
	return ok && c.Enabled
}

func ignored(path string, globs []string) bool {
	path = filepath.ToSlash(path)
	for _, g := range globs {
		g = filepath.ToSlash(g)
		if ok, _ := filepath.Match(g, path); ok {
			return true
		}
		if strings.HasPrefix(g, "**/") {
			if ok, _ := filepath.Match(g[3:], filepath.Base(path)); ok {
				return true
			}
		}
		if strings.Contains(g, "/**") {
			prefix := strings.Split(g, "/**")[0]
			if prefix != "" && (path == prefix || strings.HasPrefix(path, prefix+"/")) {
				return true
			}
		}
		if strings.HasSuffix(g, "/**") && strings.HasPrefix(path, strings.TrimSuffix(g, "**")) {
			return true
		}
	}
	return false
}

type rule struct {
	id, category, title, rationale, suggestion string
	re                                         *regexp.Regexp
	severity                                   types.Severity
	addedOnly                                  bool
}

func applyRules(f types.PackedFile, cfg types.AppConfig, checkID string, defaultSev types.Severity, rules []rule) []types.Finding {
	sev := defaultSev
	if c, ok := cfg.Checks[checkID]; ok && c.Severity != "" {
		sev = c.Severity
	}
	var out []types.Finding
	seen := map[string]bool{}
	for _, h := range f.Hunks {
		for _, ln := range h.Lines {
			if ln.Kind == "-" {
				continue
			}
			for _, r := range rules {
				if r.addedOnly && ln.Kind != "+" {
					continue
				}
				if !r.re.MatchString(ln.Text) {
					continue
				}
				line := ln.NewNo
				fp := fingerprint(checkID, f.Path, line, r.id)
				if seen[fp] {
					continue
				}
				seen[fp] = true
				s := sev
				if r.severity != "" {
					s = r.severity
				}
				out = append(out, types.Finding{
					ID:          idgen.New("fnd"),
					CheckID:     checkID,
					Category:    r.category,
					Severity:    s,
					Path:        f.Path,
					Line:        line,
					EndLine:     line,
					Side:        "RIGHT",
					Title:       r.title,
					Body:        formatBody(r.title, r.rationale, r.suggestion, checkID, s),
					Rationale:   r.rationale,
					Suggestion:  r.suggestion,
					Confidence:  0.92,
					Source:      types.SourceStatic,
					Fingerprint: fp,
				})
			}
		}
	}
	return out
}

func secrets(f types.PackedFile, cfg types.AppConfig) []types.Finding {
	return applyRules(f, cfg, "secrets", types.SeverityCritical, []rule{
		{
			id: "aws-akid", category: "secret", title: "Hardcoded AWS Access Key ID",
			rationale: "Long-lived cloud credentials in source are harvested from git history even after revert.",
			suggestion: "Move credentials to a secret manager and inject via env. Rotate this key immediately.",
			re:        regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			addedOnly: true,
		},
		{
			id: "pem", category: "secret", title: "Private key material committed",
			rationale: "PEM private keys in the repo grant whoever clones the tree equivalent access.",
			suggestion: "Remove the key, rotate it, and load from a secrets backend at runtime.",
			re:        regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`),
			addedOnly: true,
		},
		{
			id: "gh-pat", category: "secret", title: "GitHub personal access token",
			rationale: "A PAT in source is a standing credential for the installing user or bot.",
			suggestion: "Revoke the token and use GitHub App installation tokens or Actions OIDC.",
			re:        regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
			addedOnly: true,
		},
		{
			id: "slack", category: "secret", title: "Slack token or webhook",
			rationale: "Slack bot tokens and incoming webhooks in git are routinely abused for phishing.",
			suggestion: "Rotate the token and store it in env / secret manager.",
			re:        regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}|hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/`),
			addedOnly: true,
		},
		{
			id: "generic-secret-assign", category: "secret", title: "High-entropy secret assignment",
			rationale: "A quoted high-entropy literal assigned to a password/secret/token field is almost never a fixture.",
			suggestion: "Read the value from the environment. If this is a test fixture, use an obviously fake placeholder.",
			re:        regexp.MustCompile(`(?i)(password|secret|api[_-]?key|token)\s*[:=]\s*['"][^'"]{12,}['"]`),
			addedOnly: true,
		},
	})
}

func bugs(f types.PackedFile, cfg types.AppConfig) []types.Finding {
	return applyRules(f, cfg, "bugs", types.SeverityError, []rule{
		{
			id: "sql-sprintf", category: "sql-injection", title: "SQL query built with string formatting",
			rationale: "fmt.Sprintf / interpolation into SQL lets caller-controlled bytes change query structure.",
			suggestion: "Use bound parameters (db.QueryContext(ctx, `... WHERE id=?`, id)).",
			re:        regexp.MustCompile(`(?i)(fmt\.Sprintf\([^)]*(SELECT|INSERT|UPDATE|DELETE)|['"\x60].*\b(SELECT|INSERT|UPDATE|DELETE)\b.*['"\x60]\s*\+|f["'].*\b(SELECT|INSERT|UPDATE|DELETE))`),
			addedOnly: true,
		},
		{
			id: "err-underscore", category: "error-handling", title: "Error discarded with underscore",
			rationale: "Assigning err to _ drops the only signal that the operation failed.",
			suggestion: "Handle the error or wrap it. If ignore is deliberate, log at error with context.",
			re:        regexp.MustCompile(`,\s*_\s*:?=|\bif err != nil \{\s*\}`),
			addedOnly: true,
		},
		{
			id: "todo-security", category: "correctness", title: "Security TODO left in the change",
			rationale: "A security TODO in newly added code is an explicit admission the change is unfinished.",
			suggestion: "Finish the check before merge, or gate the path behind a fail-closed flag.",
			re:        regexp.MustCompile(`(?i)TODO\(?(sec|security|auth|authz)`),
			addedOnly: true,
		},
	})
}

func insecure(f types.PackedFile, cfg types.AppConfig) []types.Finding {
	return applyRules(f, cfg, "insecure", types.SeverityError, []rule{
		{
			id: "skip-verify", category: "tls", title: "TLS certificate verification disabled",
			rationale: "InsecureSkipVerify disables server authentication and enables MITM on this client.",
			suggestion: "Use a pinned CA pool or the system roots. Do not ship SkipVerify outside a local test.",
			re:        regexp.MustCompile(`InsecureSkipVerify\s*:\s*true`),
			addedOnly: true,
		},
		{
			id: "md5-sha1", category: "crypto", title: "Broken hash used in a security context",
			rationale: "MD5 and SHA-1 are collision-broken and not acceptable for tokens, checksums of untrusted input, or passwords.",
			suggestion: "Use SHA-256 or a password KDF (argon2id / bcrypt) depending on the job.",
			re:        regexp.MustCompile(`\b(md5\.(Sum|New)|sha1\.(Sum|New)|crypto\.createHash\(\s*['"]md5['"])`),
			addedOnly: true,
		},
		{
			id: "http-no-timeout", category: "availability", title: "HTTP client with no timeout",
			rationale: "Default http.Client waits forever. A hung dependency will pin goroutines and file descriptors.",
			suggestion: "Set Timeout or use a context-aware transport with an overall deadline.",
			re:        regexp.MustCompile(`&http\.Client\{\s*\}|http\.DefaultClient|http\.Get\(|http\.Post\(`),
			addedOnly: true,
		},
		{
			id: "math-rand-token", category: "crypto", title: "math/rand used for a token or secret",
			rationale: "math/rand is predictable. Session tokens generated this way can be enumerated.",
			suggestion: "Use crypto/rand.",
			re:        regexp.MustCompile(`rand\.(Intn|Read)\(.*\).*(token|secret|session)|strconv\.Itoa\(rand\.Int`),
			addedOnly: true,
		},
	})
}

func jsUnsafe(f types.PackedFile, cfg types.AppConfig) []types.Finding {
	return applyRules(f, cfg, "js-unsafe", types.SeverityError, []rule{
		{
			id: "innerhtml", category: "xss", title: "DOM sink via innerHTML",
			rationale: "Assigning untrusted HTML to innerHTML is a classic XSS sink.",
			suggestion: "Use textContent, or a sanitizer with a tight tag allowlist if HTML is required.",
			re:        regexp.MustCompile(`\.innerHTML\s*=`),
			addedOnly: true,
		},
		{
			id: "eval", category: "xss", title: "eval / Function constructor",
			rationale: "eval executes attacker-controlled strings in the caller origin.",
			suggestion: "Parse JSON with JSON.parse, or dispatch on a known command enum.",
			re:        regexp.MustCompile(`\beval\s*\(|new Function\s*\(`),
			addedOnly: true,
		},
		{
			id: "document-write", category: "xss", title: "document.write",
			rationale: "document.write can clobber the page and is an XSS sink.",
			suggestion: "Update the DOM with createElement / textContent.",
			re:        regexp.MustCompile(`document\.write\s*\(`),
			addedOnly: true,
		},
	})
}

func complexity(f types.PackedFile, cfg types.AppConfig) []types.Finding {
	threshold := 12
	if c, ok := cfg.Checks["complexity"]; ok && c.Threshold > 0 {
		threshold = c.Threshold
	}
	sev := types.SeverityWarning
	if c, ok := cfg.Checks["complexity"]; ok && c.Severity != "" {
		sev = c.Severity
	}

	type fn struct {
		name  string
		start int
		score int
	}
	var cur *fn
	flush := func(out *[]types.Finding) {
		if cur == nil || cur.score < threshold {
			cur = nil
			return
		}
		fp := fingerprint("complexity", f.Path, cur.start, cur.name)
		*out = append(*out, types.Finding{
			ID:          idgen.New("fnd"),
			CheckID:     "complexity",
			Category:    "complexity",
			Severity:    sev,
			Path:        f.Path,
			Line:        cur.start,
			EndLine:     cur.start,
			Side:        "RIGHT",
			Title:       fmt.Sprintf("High cyclomatic complexity in %s (%d)", cur.name, cur.score),
			Body:        formatBody(fmt.Sprintf("High cyclomatic complexity in %s", cur.name), fmt.Sprintf("Heuristic branch score is %d (threshold %d). Dense control flow hides missed cases and makes review expensive.", cur.score, threshold), "Split by responsibility. Extract the nested branches into named predicates or a small state machine.", "complexity", sev),
			Rationale:   "Dense control flow hides missed cases.",
			Suggestion:  "Split the function.",
			Confidence:  0.7,
			Source:      types.SourceStatic,
			Fingerprint: fp,
		})
		cur = nil
	}

	fnStart := regexp.MustCompile(`^\s*(func\s+(\([^)]+\)\s+)?([A-Za-z0-9_]+)|(?:export\s+)?(?:async\s+)?function\s+([A-Za-z0-9_]+)|(?:const|let|var)\s+([A-Za-z0-9_]+)\s*=\s*(?:async\s*)?\()`)
	branch := regexp.MustCompile(`\b(if|else if|for|while|case|&&|\|\||catch|switch)\b|\?`)

	var out []types.Finding
	for _, h := range f.Hunks {
		for _, ln := range h.Lines {
			if ln.Kind == "-" {
				continue
			}
			if m := fnStart.FindStringSubmatch(ln.Text); m != nil {
				flush(&out)
				name := firstNonEmpty(m[3], m[4], m[5], "func")
				cur = &fn{name: name, start: ln.NewNo, score: 1}
				continue
			}
			if cur != nil {
				cur.score += len(branch.FindAllString(ln.Text, -1))
				if looksLikeFnEnd(ln.Text) && braceDelta(ln.Text) < 0 {
					// keep accumulating; hunk-local heuristic is enough
				}
			}
		}
	}
	flush(&out)
	return out
}

func looksLikeFnEnd(s string) bool {
	t := strings.TrimSpace(s)
	return t == "}" || t == "};"
}

func braceDelta(s string) int {
	d := 0
	for _, r := range s {
		if r == '{' {
			d++
		}
		if r == '}' {
			d--
		}
	}
	return d
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func formatBody(title, rationale, suggestion, check string, sev types.Severity) string {
	return fmt.Sprintf("**%s** · `%s` · %s\n\n%s\n\n**Fix:** %s\n\n<sub>linejudge static · check `%s`</sub>",
		title, check, strings.ToUpper(string(sev)), rationale, suggestion, check)
}

func fingerprint(parts ...any) string {
	h := sha1.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%v\x1e", p)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func IsPrintableFindingTitle(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII && !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
