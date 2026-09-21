package fixtures

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hiroshi-os/linejudge/internal/types"
)

type Meta struct {
	Name     string `json:"name"`
	Repo     string `json:"repo"`
	PR       int    `json:"pr"`
	Title    string `json:"title"`
	SHA      string `json:"sha"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	Event    string `json:"event"`
	Action   string `json:"action"`
	Clean    bool   `json:"clean"`
	Owner    string `json:"owner"`
	RepoName string `json:"repo_name"`
}

type Fixture struct {
	Meta    Meta
	Diff    string
	Webhook []byte
	Labels  []types.LabeledIssue
	Dir     string
}

func Dir() string {
	if d := os.Getenv("LINEJUDGE_FIXTURES"); d != "" {
		return d
	}
	candidates := []string{"fixtures", "/app/fixtures"}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "fixtures"
}

func List(root string) ([]string, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(root, e.Name(), "diff.patch")); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

func Load(root, name string) (Fixture, error) {
	dir := filepath.Join(root, name)
	var fx Fixture
	fx.Dir = dir
	metaRaw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return fx, err
	}
	if err := json.Unmarshal(metaRaw, &fx.Meta); err != nil {
		return fx, err
	}
	if fx.Meta.Name == "" {
		fx.Meta.Name = name
	}
	diff, err := os.ReadFile(filepath.Join(dir, "diff.patch"))
	if err != nil {
		return fx, err
	}
	fx.Diff = string(diff)
	wh, err := os.ReadFile(filepath.Join(dir, "webhook.json"))
	if err != nil {
		return fx, fmt.Errorf("webhook.json: %w", err)
	}
	fx.Webhook = wh
	labRaw, err := os.ReadFile(filepath.Join(dir, "labels.json"))
	if err != nil {
		return fx, fmt.Errorf("labels.json: %w", err)
	}
	var wrap struct {
		Issues []types.LabeledIssue `json:"issues"`
	}
	if err := json.Unmarshal(labRaw, &wrap); err != nil {
		return fx, err
	}
	fx.Labels = wrap.Issues
	if fx.Meta.SHA == "" {
		fx.Meta.SHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}
	if fx.Meta.Event == "" {
		fx.Meta.Event = "pull_request"
	}
	if fx.Meta.Action == "" {
		fx.Meta.Action = "opened"
	}
	if fx.Meta.Base == "" {
		fx.Meta.Base = "main"
	}
	parts := strings.Split(fx.Meta.Repo, "/")
	if len(parts) == 2 {
		fx.Meta.Owner, fx.Meta.RepoName = parts[0], parts[1]
	}
	return fx, nil
}

func LoadAll(root string) ([]Fixture, error) {
	names, err := List(root)
	if err != nil {
		return nil, err
	}
	var out []Fixture
	for _, n := range names {
		fx, err := Load(root, n)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out = append(out, fx)
	}
	return out, nil
}
