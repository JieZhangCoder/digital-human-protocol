package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openkursar/digital-human-protocol/server/internal/config"
	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

const validSpec = `
spec_version: "1"
name: Test Agent
version: "1.0.0"
author: alice
description: Performs a useful daily task.
type: automation
system_prompt: |
  Do work and report back.
subscriptions:
  - source:
      type: schedule
      config: { every: "1h" }
store:
  slug: alice/test-agent
  tags: [test]
  license: MIT
permissions: [ai-browser]
`

func newTestServer(t *testing.T) (*Server, *storage.Local) {
	t.Helper()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	cfg := &config.Config{
		Listen: ":0",
		Storage: config.StorageBlock{Type: config.StorageLocal},
		Auth: config.AuthBlock{Type: "token"},
		MaxUploadBytes: 1 << 20,
		RequestTimeout: 5 << 30,
	}
	cfg.Rules.Enabled = []string{"schema_valid", "dangerous_permissions"}
	return NewServer(cfg, store), store
}

func TestHealthz(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestPublishAndFetch(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	w, err := mw.CreateFormFile("spec", "spec.yaml")
	if err != nil {
		t.Fatalf("part: %v", err)
	}
	io.WriteString(w, validSpec)
	mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/apps", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body=%s", resp.StatusCode, string(raw))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["slug"] != "alice/test-agent" {
		t.Fatalf("bad slug: %v", out)
	}

	// Index should now list the agent.
	resp2, err := http.Get(ts.URL + "/digital-humans.json")
	if err != nil {
		t.Fatalf("get index: %v", err)
	}
	defer resp2.Body.Close()
	idx, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(idx), "alice/test-agent") {
		t.Fatalf("index missing entry: %s", idx)
	}
}

func TestPublishBlocksFailingSpec(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	bad := `
name: ""
version: "1.0.0"
author: a
description: d
type: automation
system_prompt: x
subscriptions:
  - source: { type: schedule, config: { every: "1h" } }
`
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	w, _ := mw.CreateFormFile("spec", "spec.yaml")
	io.WriteString(w, bad)
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/apps", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 (validate), got %d", resp.StatusCode)
	}
}

func TestFetchArtifact(t *testing.T) {
	srv, store := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Seed the store directly so we don't need a full publish flow.
	key := "apps/alice/hello/1.0.0/files/SKILL.md"
	_ = store.Put(nil, key, strings.NewReader("hello"), 5)

	resp, err := http.Get(ts.URL + "/apps/alice/hello/1.0.0/files/SKILL.md")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Fatalf("body=%q", body)
	}
}

func TestFetchArtifactMissing(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/apps/x/1.0.0/files/missing")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestLegacyIndex(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/index.json")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "deprecated") {
		t.Fatalf("missing deprecation notice: %s", body)
	}
}

func TestAuthEnforced(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg.Auth.Token = "secret"
	srv.auth = nil // force rebuild via Handler() path
	srv = NewServer(srv.cfg, srv.store)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/apps", bytes.NewReader(nil))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}
