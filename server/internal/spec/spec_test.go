package spec

import (
	"errors"
	"strings"
	"testing"
)

const minimalAutomation = `
spec_version: "1"
name: Test
version: "1.0.0"
author: alice
description: does something
type: automation
system_prompt: do work
subscriptions:
  - source:
      type: schedule
      config:
        every: "1h"
`

func TestParseMinimal(t *testing.T) {
	s, err := Parse([]byte(minimalAutomation))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Name != "Test" || s.Type != TypeAutomation {
		t.Fatalf("unexpected spec: %+v", s)
	}
	if err := Validate(s); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestSubscriptionShorthand(t *testing.T) {
	short := `
name: T
version: "1.0.0"
author: a
description: d
type: automation
system_prompt: x
subscriptions:
  - type: schedule
    config: { every: "1h" }
`
	s, err := Parse([]byte(short))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Subscriptions[0].Source == nil || s.Subscriptions[0].Source.Type != "schedule" {
		t.Fatalf("shorthand not normalized: %+v", s.Subscriptions[0])
	}
}

func TestValidateMissingFields(t *testing.T) {
	cases := map[string]string{
		"no name": `
version: "1.0.0"
author: a
description: d
type: skill
system_prompt: x
`,
		"skill must not have subs": `
name: x
version: "1.0.0"
author: a
description: d
type: skill
system_prompt: x
subscriptions:
  - source: { type: schedule, config: { every: "1h" } }
`,
		"mcp needs command": `
name: x
version: "1.0.0"
author: a
description: d
type: mcp
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			s, err := Parse([]byte(body))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = Validate(s)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			var ve *ValidationError
			if !errors.As(err, &ve) || len(ve.Issues) == 0 {
				t.Fatalf("expected *ValidationError, got %T", err)
			}
		})
	}
}

func TestDeriveSlug(t *testing.T) {
	cases := map[string]string{
		"hn-daily":               "hn-daily",
		"HN Daily":               "hn-daily",
		"  My  App  ":            "my-app",
		"Foo!!! Bar???":          "foo-bar",
		"Already-Slug":           "already-slug",
		"foo--bar":               "foo-bar",
		"---weird---":            "weird",
		"v2.1 release":           "v2-1-release",
		"中文名":                    "", // no ASCII alphanumerics; caller must error
		"中文 mixed Name":          "mixed-name",
	}
	for in, want := range cases {
		if got := DeriveSlug(in); got != want {
			t.Errorf("DeriveSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugFallsBackToDerivation(t *testing.T) {
	body := `
name: HN Daily
version: "1.0.0"
author: a
description: d
type: automation
system_prompt: x
`
	s, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := s.Slug(); got != "hn-daily" {
		t.Fatalf("Slug() = %q, want %q (derived from name)", got, "hn-daily")
	}
}

func TestSlugFormat(t *testing.T) {
	good := []string{"foo", "foo-bar", "alice/foo-bar"}
	bad := []string{"-bad", "bad-", "Foo", "alice/Bad"}
	for _, g := range good {
		if !scopedSlugRe.MatchString(g) {
			t.Errorf("want valid: %q", g)
		}
	}
	for _, b := range bad {
		if scopedSlugRe.MatchString(b) {
			t.Errorf("want invalid: %q", b)
		}
	}
}

func TestSkillDependencyShorthand(t *testing.T) {
	body := `
name: T
version: "1.0.0"
author: a
description: d
type: automation
system_prompt: x
subscriptions:
  - source: { type: schedule, config: { every: "1h" } }
requires:
  skills:
    - summarizer
    - id: openkursar/xhs-search
      version: "^2.1"
`
	s, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(s.Requires.Skills) != 2 {
		t.Fatalf("got %d skills", len(s.Requires.Skills))
	}
	if s.Requires.Skills[0].ID != "summarizer" {
		t.Fatalf("shorthand not handled: %+v", s.Requires.Skills[0])
	}
	if !strings.Contains(s.Requires.Skills[1].ID, "/") {
		t.Fatalf("scoped id lost: %+v", s.Requires.Skills[1])
	}
}
