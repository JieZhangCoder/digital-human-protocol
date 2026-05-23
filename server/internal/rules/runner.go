// Package rules holds the review rule implementations and the aggregator that
// runs them in order.
//
// Each rule is implemented as a free function returning a Verdict so they can
// be composed (and unit-tested) without instantiating a struct per rule. The
// Runner produces a Report — the canonical artefact written by the CLI and
// returned by the registry's /apps endpoint.
package rules

import (
	"context"
	"fmt"
	"sort"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
	"github.com/openkursar/digital-human-protocol/server/internal/config"
	"github.com/openkursar/digital-human-protocol/server/internal/spec"
)

// Severity ranks rule outcomes from best to worst.
type Severity int

const (
	SeverityPass Severity = iota
	SeverityWarn
	SeverityFail
	SeverityError
)

// String renders the severity in the lower-case form used by JSON output.
func (s Severity) String() string {
	switch s {
	case SeverityPass:
		return "pass"
	case SeverityWarn:
		return "warn"
	case SeverityFail:
		return "fail"
	case SeverityError:
		return "error"
	}
	return "unknown"
}

// Verdict is the per-rule outcome.
type Verdict struct {
	Rule          string   `json:"rule"`
	Severity      Severity `json:"-"`
	SeverityText  string   `json:"severity"`
	Message       string   `json:"message"`
	Details       []string `json:"details,omitempty"`
	RequiresHuman bool     `json:"requires_human,omitempty"`
	Skipped       bool     `json:"skipped,omitempty"`
}

// Report is the aggregated result of a review run.
type Report struct {
	Slug          string    `json:"slug"`
	Version       string    `json:"version"`
	Type          string    `json:"type"`
	Verdicts      []Verdict `json:"verdicts"`
	Overall       Severity  `json:"-"`
	OverallText   string    `json:"overall"`
	RequiresHuman bool      `json:"requires_human"`
}

// Options bundles everything a rule may need at evaluation time.
type Options struct {
	Spec        *spec.Spec
	SpecBytes   []byte
	SkillFiles  map[string][]byte // path -> contents (for skill_code_safety)
	Config      *config.Config
	AI          *ai.Client
	Registry    RegistryLookup
	RawRegistry []map[string]any // for duplicate_detection context
}

// RegistryLookup is the minimal interface rules need to query the registry
// index. It is split out so the CLI can pass an HTTP-backed implementation and
// tests can pass an in-memory fake.
type RegistryLookup interface {
	// LookupSlug returns the existing entry for slug (or nil + ErrNotFound).
	LookupSlug(ctx context.Context, slug string) (*RegistryEntry, error)
}

// RegistryEntry is a partial mirror of the registry index entry.
type RegistryEntry struct {
	Slug    string
	Version string
	Author  string
	Name    string
}

// CheckFunc is the signature of every review rule.
type CheckFunc func(ctx context.Context, opts *Options) Verdict

// registry maps rule names to their implementations.
var registry = map[string]CheckFunc{}

// Register adds a rule. Panics on duplicate registration — these are package
// init globals, not user input.
func Register(name string, fn CheckFunc) {
	if _, ok := registry[name]; ok {
		panic(fmt.Sprintf("rules: duplicate registration for %q", name))
	}
	registry[name] = fn
}

// Names returns the registered rule names sorted alphabetically.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Run executes the enabled rules in a stable order and produces a Report.
func Run(ctx context.Context, opts *Options) *Report {
	r := &Report{}
	if opts == nil || opts.Spec == nil {
		r.Verdicts = append(r.Verdicts, Verdict{
			Rule:         "runner",
			Severity:     SeverityError,
			SeverityText: SeverityError.String(),
			Message:      "no spec supplied to runner",
		})
		r.Overall = SeverityError
		r.OverallText = r.Overall.String()
		return r
	}
	if opts.Spec.Store != nil {
		r.Slug = opts.Spec.Store.Slug
	}
	r.Version = opts.Spec.Version
	r.Type = string(opts.Spec.Type)

	for _, name := range orderedRules(opts.Config) {
		fn, ok := registry[name]
		if !ok {
			continue
		}
		if opts.Config != nil && !opts.Config.IsRuleEnabled(name) {
			r.Verdicts = append(r.Verdicts, Verdict{
				Rule:         name,
				Severity:     SeverityPass,
				SeverityText: SeverityPass.String(),
				Message:      "rule disabled by config",
				Skipped:      true,
			})
			continue
		}
		v := fn(ctx, opts)
		v.Rule = name
		v.SeverityText = v.Severity.String()
		if v.Severity > r.Overall {
			r.Overall = v.Severity
		}
		if v.RequiresHuman {
			r.RequiresHuman = true
		}
		r.Verdicts = append(r.Verdicts, v)
	}
	r.OverallText = r.Overall.String()
	return r
}

// orderedRules returns names in execution order. Schema validation runs first
// (blocking failures here short-circuit the rest? No — we run everything but
// the overall severity will reflect the worst outcome).
func orderedRules(cfg *config.Config) []string {
	canonical := []string{
		"schema_valid",
		"slug_unique",
		"dangerous_permissions",
		"sensitive_categories",
		"skill_code_safety",
		"prompt_quality",
		"metadata_compliance",
		"duplicate_detection",
	}
	if cfg == nil {
		return canonical
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(canonical))
	for _, n := range canonical {
		if !seen[n] {
			out = append(out, n)
			seen[n] = true
		}
	}
	for _, n := range cfg.Rules.Enabled {
		if !seen[n] {
			out = append(out, n)
			seen[n] = true
		}
	}
	return out
}
