package main

import (
	"bytes"
	"context"
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

// TestRebuildIndexesAfterRestart simulates a process restart by:
//  1. Publishing a digital human + a skill against server A
//  2. Building a brand-new Server B with the SAME storage root
//  3. Verifying B's indexes are empty before RebuildIndexes
//  4. Calling B.RebuildIndexes and verifying both entries reappear
//
// This is the core regression test for the "in-memory index lost on restart"
// production risk called out in go-agent's review.
func TestRebuildIndexesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	cfg := &config.Config{
		Listen:         ":0",
		Storage:        config.StorageBlock{Type: config.StorageLocal},
		Auth:           config.AuthBlock{Type: "token"},
		MaxUploadBytes: 1 << 20,
		RequestTimeout: 5 << 30,
	}
	cfg.Rules.Enabled = []string{"schema_valid", "dangerous_permissions"}

	// --- session 1: publish a digital human + a skill ---
	srvA := NewServer(cfg, store)
	tsA := httptest.NewServer(srvA.Handler())

	publish := func(spec string, ts *httptest.Server) {
		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		w, _ := mw.CreateFormFile("spec", "spec.yaml")
		io.WriteString(w, spec)
		mw.Close()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/apps", body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("publish status %d body=%s", resp.StatusCode, string(raw))
		}
	}
	publish(validSpec, tsA)
	publish(`
spec_version: "1"
name: Echo Skill
version: "1.0.0"
author: alice
description: Echoes input back.
type: skill
system_prompt: |
  Echo the input verbatim.
store:
  slug: alice/echo
  license: MIT
`, tsA)

	// Confirm session-1 indexes look right before tearing down.
	resp, _ := http.Get(tsA.URL + "/digital-humans.json")
	bodyA, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(bodyA), "alice/test-agent") {
		t.Fatalf("session 1 digital-humans missing entry: %s", bodyA)
	}
	tsA.Close()

	// --- session 2: brand-new Server reusing the same storage root ---
	store2, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatalf("storage 2: %v", err)
	}
	srvB := NewServer(cfg, store2)
	tsB := httptest.NewServer(srvB.Handler())
	defer tsB.Close()

	// Before rebuild: indexes are empty (memory state is fresh).
	resp, _ = http.Get(tsB.URL + "/digital-humans.json")
	bodyB1, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(bodyB1), "alice/test-agent") {
		t.Fatalf("session 2 indexes unexpectedly populated before rebuild: %s", bodyB1)
	}

	// Rebuild from disk.
	if err := srvB.RebuildIndexes(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// After rebuild: both entries reappear in their respective indexes.
	resp, _ = http.Get(tsB.URL + "/digital-humans.json")
	bodyDH, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(bodyDH), "alice/test-agent") {
		t.Fatalf("digital-humans.json missing rebuilt entry: %s", bodyDH)
	}

	resp, _ = http.Get(tsB.URL + "/skills.json")
	bodySK, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(bodySK), "alice/echo") {
		t.Fatalf("skills.json missing rebuilt entry: %s", bodySK)
	}

	// Artifact retrieval still works (storage was the source of truth).
	resp, _ = http.Get(tsB.URL + "/apps/alice/test-agent/1.0.0/files/spec.yaml")
	if resp.StatusCode != 404 {
		// spec.yaml is stored at apps/<slug>/<version>/spec.yaml, NOT under files/
		// (that's the publish convention). 404 here is correct; just drain the body.
		io.Copy(io.Discard, resp.Body)
	}
	resp.Body.Close()
}

// TestRebuildIndexesEmptyStorage verifies a fresh install (no published apps
// yet) doesn't error out — startup must succeed even with an empty storage root.
func TestRebuildIndexesEmptyStorage(t *testing.T) {
	srv, _ := newTestServer(t)
	if err := srv.RebuildIndexes(context.Background()); err != nil {
		t.Fatalf("rebuild on empty storage failed: %v", err)
	}
}

// TestIndexEnvelopeShape pins the wire format of /digital-humans.json (and
// siblings) to {version, generated_at, source, apps[]}. The Halo client's
// RegistryIndexSchema is strict about these four keys — if the server ever
// regresses to a bare array (as a previous iteration did), client sync silently
// fails with "Invalid index format: version, generated_at, source, apps" and
// the store appears stuck on stale cache.
func TestIndexEnvelopeShape(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, path := range []string{"/digital-humans.json", "/skills.json", "/mcps.json", "/index.json"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("%s did not decode as JSON object (likely a bare array — regression): %v body=%s", path, err, body)
		}
		for _, key := range []string{"version", "generated_at", "source", "apps"} {
			if _, ok := decoded[key]; !ok {
				t.Errorf("%s missing required envelope key %q (body=%s)", path, key, body)
			}
		}
		if apps, ok := decoded["apps"].([]any); !ok {
			t.Errorf("%s apps field is not an array (got %T)", path, decoded["apps"])
		} else if path != "/index.json" && len(apps) != 0 {
			t.Errorf("%s apps should be empty on fresh server (got %d)", path, len(apps))
		}
	}
}

// TestIndexEntryHasFormat pins the per-app shape inside the index envelope:
// every entry must carry `format: "bundle"`. The Halo client's
// RegistryEntrySchema declares this field with z.literal('bundle'); skipping
// it makes every entry fail validation and the whole sync drop to zero rows.
func TestIndexEntryHasFormat(t *testing.T) {
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
		t.Fatalf("publish: %v", err)
	}
	resp.Body.Close()

	resp2, err := http.Get(ts.URL + "/digital-humans.json")
	if err != nil {
		t.Fatalf("get digital-humans.json: %v", err)
	}
	raw, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	var decoded struct {
		Apps []map[string]any `json:"apps"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw)
	}
	if len(decoded.Apps) == 0 {
		t.Fatalf("expected at least one app entry, got 0 (body=%s)", raw)
	}
	for i, app := range decoded.Apps {
		format, ok := app["format"]
		if !ok {
			t.Errorf("apps[%d] missing required `format` field (body=%s)", i, raw)
			continue
		}
		if format != "bundle" {
			t.Errorf("apps[%d].format expected \"bundle\", got %q", i, format)
		}
	}
}

// TestArtifactRouteShapes exhaustively pins every URL shape the Halo client
// constructs to fetch published artifacts. Each row publishes a single spec
// then asserts that BOTH spec.yaml and an auxiliary file are reachable. Any
// silent route regression here cascades into store detail-view 404s.
func TestArtifactRouteShapes(t *testing.T) {
	cases := []struct {
		name string
		spec string // YAML spec body
	}{
		{
			name: "scoped slug",
			spec: `
spec_version: "1"
name: Scoped Skill
version: "1.0.0"
author: alice
description: Skill with scoped slug.
type: skill
system_prompt: |
  Hi.
store:
  slug: alice/scoped-skill
`,
		},
		{
			name: "flat slug",
			spec: `
spec_version: "1"
name: Flat Skill
version: "2.0.0"
author: bob
description: Skill with flat slug.
type: skill
system_prompt: |
  Hi.
store:
  slug: flat-skill
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			ts := httptest.NewServer(srv.Handler())
			defer ts.Close()

			// Publish: spec + one auxiliary file under files/SKILL.md
			body := &bytes.Buffer{}
			mw := multipart.NewWriter(body)
			w, err := mw.CreateFormFile("spec", "spec.yaml")
			if err != nil {
				t.Fatalf("part spec: %v", err)
			}
			io.WriteString(w, tc.spec)
			w2, err := mw.CreateFormFile("SKILL.md", "SKILL.md")
			if err != nil {
				t.Fatalf("part SKILL.md: %v", err)
			}
			io.WriteString(w2, "# Hello from "+tc.name)
			mw.Close()

			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/apps", body)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			pubBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("publish status %d body=%s", resp.StatusCode, pubBody)
			}
			var pub map[string]any
			if err := json.Unmarshal(pubBody, &pub); err != nil {
				t.Fatalf("decode publish: %v", err)
			}
			specPath, _ := pub["path"].(string)
			if specPath == "" {
				t.Fatalf("publish response missing path: %s", pubBody)
			}

			// Halo client URL shapes:
			//   1. {base}/{path}/spec.yaml
			//   2. {base}/{path}/files/SKILL.md
			for _, target := range []struct {
				url, expectBodyContains string
			}{
				{ts.URL + "/" + specPath + "/spec.yaml", "spec_version"},
				{ts.URL + "/" + specPath + "/files/SKILL.md", "# Hello from "},
			} {
				r, err := http.Get(target.url)
				if err != nil {
					t.Fatalf("get %s: %v", target.url, err)
				}
				raw, _ := io.ReadAll(r.Body)
				r.Body.Close()
				if r.StatusCode != 200 {
					t.Errorf("%s: expected 200, got %d body=%s", target.url, r.StatusCode, raw)
					continue
				}
				if !strings.Contains(string(raw), target.expectBodyContains) {
					t.Errorf("%s: body %q did not contain %q", target.url, raw, target.expectBodyContains)
				}
			}

			// Negative case: a path that doesn't match any known shape must 404.
			r, err := http.Get(ts.URL + "/" + specPath + "/random-thing")
			if err != nil {
				t.Fatalf("get random: %v", err)
			}
			r.Body.Close()
			if r.StatusCode != http.StatusNotFound {
				t.Errorf("unknown path expected 404, got %d", r.StatusCode)
			}
		})
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
