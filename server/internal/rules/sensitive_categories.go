package rules

import (
	"encoding/json"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
)

// SensitiveCategoriesRule uses the AI judge to flag sensitive domains.
type SensitiveCategoriesRule struct {
	judge *ai.Judge
}

// NewSensitiveCategoriesRule creates the rule.
func NewSensitiveCategoriesRule(judge *ai.Judge) *SensitiveCategoriesRule {
	return &SensitiveCategoriesRule{judge: judge}
}

// Name returns the rule identifier.
func (r *SensitiveCategoriesRule) Name() string { return "sensitive_categories" }

// Evaluate asks the AI whether the spec belongs to a sensitive category.
func (r *SensitiveCategoriesRule) Evaluate(inp *Input) *Result {
	specJSON, _ := json.Marshal(inp.Spec)
	res, err := r.judge.Review(
		"You classify AI skills by sensitivity. Only flag clearly sensitive domains.",
		"Does this belong to a sensitive category (finance, medical, legal)? Return JSON {'verdict':'approved|needs_review','comment':'...'}.\nSpec: "+string(specJSON),
	)
	if err != nil {
		return &Result{Rule: r.Name(), Passed: false, Block: false, Manual: true, Comment: err.Error()}
	}
	manual := res.Verdict == "needs_review"
	return &Result{
		Rule:    r.Name(),
		Passed:  !manual,
		Block:   false,
		Manual:  manual,
		Comment: res.Comment,
	}
}
