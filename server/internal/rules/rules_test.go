package rules

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
	"github.com/openkursar/digital-human-protocol/server/internal/config"
	"github.com/openkursar/digital-human-protocol/server/internal/spec"
	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

const validAutomation = `
spec_version: "1"
name: HN Daily
version: "1.0.0"
author: alice
description: Summarises HN news every morning.
type: automation
system_prompt: |
  Visit HN and summarise.
subscriptions:
  - source:
      type: schedule
      config: { every: "1h" }
store:
  slug: alice/hn
  tags: [news, hn]
`

func loadSpec(t *testing.T, body string) *spec.Spec {
	t.Helper()
	s, err := spec.Parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return s
}

func newConfig() *config.Config {
	c := &config.Config{}
	// Simulate Load() defaults.
	c.Rules.Enabled = []string{"schema_valid", "slug_unique", "dangerous_permissions", "sensitive_categories"}
	c.Rules.SensitiveKeywords = []string{"financial advice"}
	return c
}

type fakeRegistry struct{ entries map[string]*RegistryEntry }

func (f *fakeRegistry) LookupSlug(_ context.Context, slug string) (*RegistryEntry, error) {
	if f == nil {
		return nil, storage.ErrNotFound
	}
	if e, ok := f.entries[slug]; ok {
		return e, nil
	}
	return nil, storage.ErrNotFound
}

func TestSchemaValid(t *testing.T) {
	s := loadSpec(t, validAutomation)
	v := checkSchemaValid(context.Background(), &Options{Spec: s})
	if v.Severity != SeverityPass {
		t.Fatalf("want pass, got %s: %s", v.Severity, v.Message)
	}
}

func TestSchemaInvalid(t *testing.T) {
	bad := `
name: ""
type: automation
version: "1.0.0"
author: a
description: d
system_prompt: x
subscriptions:
  - source: { type: schedule, config: { every: "1h" } }
`
	s := loadSpec(t, bad)
	v := checkSchemaValid(context.Background(), &Options{Spec: s})
	if v.Severity != SeverityFail {
		t.Fatalf("want fail, got %s", v.Severity)
	}
}

func TestSlugUnique(t *testing.T) {
	s := loadSpec(t, validAutomation)
	reg := &fakeRegistry{entries: map[string]*RegistryEntry{
		"alice/hn": {Slug: "alice/hn", Version: "0.9.0"},
	}}
	v := checkSlugUnique(context.Background(), &Options{Spec: s, Registry: reg})
	if v.Severity != SeverityPass {
		t.Fatalf("want pass on upgrade, got %s: %s", v.Severity, v.Message)
	}
	// Same version → warn
	reg.entries["alice/hn"].Version = "1.0.0"
	v = checkSlugUnique(context.Background(), &Options{Spec: s, Registry: reg})
	if v.Severity != SeverityWarn {
		t.Fatalf("want warn on same version, got %s", v.Severity)
	}
	// Higher in registry → fail (downgrade attempt)
	reg.entries["alice/hn"].Version = "2.0.0"
	v = checkSlugUnique(context.Background(), &Options{Spec: s, Registry: reg})
	if v.Severity != SeverityFail {
		t.Fatalf("want fail on downgrade, got %s", v.Severity)
	}
}

func TestDangerousPermissions(t *testing.T) {
	s := loadSpec(t, validAutomation)
	s.Permissions = []string{"ai-browser", "shell-exec"}
	c := newConfig()
	v := checkDangerousPermissions(context.Background(), &Options{Spec: s, Config: c})
	if v.Severity != SeverityFail {
		t.Fatalf("want fail, got %s", v.Severity)
	}

	// Now allowlist it.
	c.Rules.Allowlist.DangerousPermissions = []string{"shell-exec"}
	v = checkDangerousPermissions(context.Background(), &Options{Spec: s, Config: c})
	if v.Severity != SeverityPass {
		t.Fatalf("want pass after allowlist, got %s", v.Severity)
	}
}

func TestSensitiveCategories(t *testing.T) {
	s := loadSpec(t, validAutomation)
	s.Description = "Provides financial advice on Tesla stock."
	c := newConfig()
	v := checkSensitiveCategories(context.Background(), &Options{Spec: s, Config: c})
	if v.Severity != SeverityWarn || !v.RequiresHuman {
		t.Fatalf("want warn+requires_human, got %s rh=%t", v.Severity, v.RequiresHuman)
	}
}

func TestRunnerAggregatesWorst(t *testing.T) {
	s := loadSpec(t, validAutomation)
	s.Permissions = []string{"shell-exec"}
	c := newConfig()
	rep := Run(context.Background(), &Options{Spec: s, Config: c})
	if rep.Overall != SeverityFail {
		t.Fatalf("want overall fail, got %s", rep.Overall)
	}
	if rep.Slug != "alice/hn" {
		t.Fatalf("slug not propagated: %q", rep.Slug)
	}
}

func TestRenderMarkdown(t *testing.T) {
	s := loadSpec(t, validAutomation)
	c := newConfig()
	rep := Run(context.Background(), &Options{Spec: s, Config: c})
	md := RenderMarkdown(rep)
	if !strings.Contains(md, "DHP Review Report") {
		t.Fatalf("markdown missing header: %s", md)
	}
}

func TestSlugUniqueNoRegistry(t *testing.T) {
	s := loadSpec(t, validAutomation)
	v := checkSlugUnique(context.Background(), &Options{Spec: s})
	if v.Severity != SeverityWarn {
		t.Fatalf("want warn when no registry, got %s", v.Severity)
	}
}

func TestRunnerHandlesNilSpec(t *testing.T) {
	rep := Run(context.Background(), &Options{})
	if rep.Overall != SeverityError {
		t.Fatalf("want error when no spec, got %s", rep.Overall)
	}
}

func TestRenderJSON(t *testing.T) {
	s := loadSpec(t, validAutomation)
	c := newConfig()
	rep := Run(context.Background(), &Options{Spec: s, Config: c})
	out, err := RenderJSON(rep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"slug": "alice/hn"`) {
		t.Fatalf("missing slug in json: %s", out)
	}
}

func TestNames(t *testing.T) {
	got := Names()
	want := map[string]bool{
		"schema_valid": true, "slug_unique": true, "dangerous_permissions": true,
		"sensitive_categories": true, "skill_code_safety": true,
		"prompt_quality": true, "metadata_compliance": true, "duplicate_detection": true,
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected rule registered: %s", n)
		}
	}
	for n := range want {
		found := false
		for _, g := range got {
			if g == n {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing rule: %s", n)
		}
	}
}

func TestAIJudgesDisabled(t *testing.T) {
	s := loadSpec(t, validAutomation)
	// prompt_quality and metadata_compliance always call the AI client, so
	// when the client is disabled they should fall through to Warn.
	for _, fn := range []CheckFunc{checkPromptQuality, checkMetadataCompliance} {
		v := fn(context.Background(), &Options{Spec: s, AI: ai.New("", "", "")})
		if v.Severity != SeverityWarn {
			t.Fatalf("disabled AI should yield warn, got %s: %s", v.Severity, v.Message)
		}
	}
}

func TestAIJudgesPass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(ai.Response{Verdict: ai.VerdictPass, Reason: "looks good"})
	}))
	defer ts.Close()
	s := loadSpec(t, validAutomation)
	v := checkPromptQuality(context.Background(), &Options{Spec: s, AI: ai.New(ts.URL, "m", "")})
	if v.Severity != SeverityPass {
		t.Fatalf("want pass from AI, got %s: %s", v.Severity, v.Message)
	}
}

func TestAIJudgeFailRequiresHuman(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(ai.Response{Verdict: ai.VerdictFail, Reason: "looks malicious"})
	}))
	defer ts.Close()
	s := loadSpec(t, validAutomation)
	v := checkMetadataCompliance(context.Background(), &Options{Spec: s, AI: ai.New(ts.URL, "m", "")})
	if v.Severity != SeverityFail || !v.RequiresHuman {
		t.Fatalf("want fail+requires_human, got %s rh=%t", v.Severity, v.RequiresHuman)
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"2.0", "1.9.9", 1},
		{"1.0-beta", "1.0", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
