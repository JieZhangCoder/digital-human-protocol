package rules

import (
	"github.com/openkursar/digital-human-protocol/server/internal/ai"
	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

// Result is the outcome of one rule evaluation.
type Result struct {
	Rule    string `json:"rule"`
	Passed  bool   `json:"passed"`
	Block   bool   `json:"block"`
	Manual  bool   `json:"manual"`
	Comment string `json:"comment"`
}

// Input holds everything a rule needs.
type Input struct {
	Slug     string
	Version  string
	Spec     map[string]interface{}
	Files    map[string][]byte
	Existing []storage.AppRef
}

// Rule is a single review rule.
type Rule interface {
	Name() string
	Evaluate(inp *Input) *Result
}

// Engine runs enabled rules.
type Engine struct {
	rules []Rule
}

// NewEngine builds the engine with only enabled rules.
func NewEngine(enabled []string, judge *ai.Judge) *Engine {
	allRules := []Rule{
		&SchemaValidRule{},
		&SlugUniqueRule{},
		&DangerousPermissionsRule{},
		NewSkillCodeSafetyRule(judge),
		NewPromptQualityRule(judge),
		NewMetadataComplianceRule(judge),
		NewDuplicateDetectionRule(judge),
		NewSensitiveCategoriesRule(judge),
	}
	enabledSet := make(map[string]bool)
	for _, name := range enabled {
		enabledSet[name] = true
	}
	var filtered []Rule
	for _, r := range allRules {
		if enabledSet[r.Name()] {
			filtered = append(filtered, r)
		}
	}
	return &Engine{rules: filtered}
}

// Run evaluates all enabled rules against the input.
func (e *Engine) Run(inp *Input) []*Result {
	var results []*Result
	for _, r := range e.rules {
		results = append(results, r.Evaluate(inp))
	}
	return results
}

// Verdict aggregates results into approved / needs_review / rejected.
func Verdict(results []*Result, threshold string) string {
	for _, r := range results {
		if r.Block {
			return "rejected"
		}
	}
	for _, r := range results {
		if r.Manual {
			return "needs_review"
		}
	}
	return "approved"
}

// CommentFromResults returns the first failing comment or a success message.
func CommentFromResults(results []*Result) string {
	for _, r := range results {
		if !r.Passed {
			return r.Comment
		}
	}
	return "All checks passed"
}
