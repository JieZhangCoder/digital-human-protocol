package rules

import (
	"github.com/openkursar/digital-human-protocol/server/internal/ai"
)

// PromptQualityRule uses the AI judge to rate description quality.
type PromptQualityRule struct {
	judge *ai.Judge
}

// NewPromptQualityRule creates the rule.
func NewPromptQualityRule(judge *ai.Judge) *PromptQualityRule {
	return &PromptQualityRule{judge: judge}
}

// Name returns the rule identifier.
func (r *PromptQualityRule) Name() string { return "prompt_quality" }

// Evaluate asks the AI to rate the description.
func (r *PromptQualityRule) Evaluate(inp *Input) *Result {
	desc := ""
	if d, ok := inp.Spec["description"].(string); ok {
		desc = d
	}
	res, err := r.judge.Review(
		"You are a prompt quality reviewer. Check clarity, completeness, safety.",
		"Rate this description. Return JSON {'verdict':'approved|rejected|needs_review','comment':'...'}.\nDescription: "+desc,
	)
	if err != nil {
		return &Result{Rule: r.Name(), Passed: false, Block: true, Comment: err.Error()}
	}
	return &Result{
		Rule:    r.Name(),
		Passed:  res.Verdict == "approved",
		Block:   res.Verdict == "rejected",
		Comment: res.Comment,
	}
}
