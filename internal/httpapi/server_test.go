package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hiroshi-os/linejudge/internal/httpapi"
	"github.com/hiroshi-os/linejudge/internal/store"
	"github.com/hiroshi-os/linejudge/internal/worker"
)

func TestWebhookFixtureReview(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(root, "fixtures", "secret-leak", "diff.patch")); err == nil {
			break
		}
		root = filepath.Dir(root)
	}
	if err := httpapi.SeedDemoRepos(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	cfg, _ := st.GetConfig(context.Background())
	c := cfg.Checks["llm-assist"]
	c.Approve = true
	cfg.Checks["llm-assist"] = c
	_ = st.SaveConfig(context.Background(), cfg)

	w := worker.New(st)
	srv := httpapi.New(st, filepath.Join(root, "fixtures"))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	payload, err := os.ReadFile(filepath.Join(root, "fixtures", "secret-leak", "webhook.json"))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "test-delivery-secret-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	var out struct {
		Created bool `json:"created"`
		Review  struct {
			ID string `json:"id"`
		} `json:"review"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Created || out.Review.ID == "" {
		t.Fatalf("enqueue: %s", body)
	}
	if err := w.Drain(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	rev, _, err := st.GetReview(context.Background(), out.Review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Status != "completed" {
		t.Fatalf("status %s err=%s", rev.Status, rev.Error)
	}
	comments, err := st.ListComments(context.Background(), out.Review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) == 0 {
		t.Fatal("expected review comments")
	}
	if comments[0].Path == "" || comments[0].Line == 0 || comments[0].Body == "" {
		t.Fatalf("github shape %+v", comments[0])
	}

	// idempotent redelivery
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/webhooks/github", bytes.NewReader(payload))
	req2.Header.Set("X-GitHub-Event", "pull_request")
	req2.Header.Set("X-GitHub-Delivery", "test-delivery-secret-1")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if !bytes.Contains(b2, []byte(`"idempotent": true`)) {
		t.Fatalf("expected idempotent, got %s", b2)
	}
}
