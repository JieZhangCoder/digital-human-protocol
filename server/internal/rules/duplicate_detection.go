package rules

import (
	"encoding/json"
	"strings"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
)

// DuplicateDetectionRule uses the AI judge to flag duplicates.
type DuplicateDetectionRule struct {
	judge *ai.Judge
}

// NewDuplicateDetectionRule creates the rule.
func NewDuplicateDetectionRule(judge *ai.Judge) *DuplicateDetectionRule {
	return &DuplicateDetectionRule{judge: judge}
}

// Name returns the rule identifier.
func (r *DuplicateDetectionRule) Name() string { return "duplicate_detection" }

// Evaluate asks the AI whether this spec duplicates existing ones.
func (r *DuplicateDetectionRule) Evaluate(inp *Input) *Result {
	var existing []string
	for _, ref := range inp.Existing {
		existing = append(existing, ref.Slug)
	}
	specJSON, _ := json.Marshal(inp.Spec)
	res, err := r.judge.Review(
		"You detect duplicate AI skills. Flag only clear duplicates.",
		"Does this duplicate existing ones? Existing: ["+strings.Join(existing, ", ")+"]. Return JSON {'verdict':'approved|rejected|needs_review','comment':'...'}.\nSpec: "+string(specJSON),
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
