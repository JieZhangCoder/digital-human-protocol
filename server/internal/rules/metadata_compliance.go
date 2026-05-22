package rules

import (
	"encoding/json"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
)

// MetadataComplianceRule uses the AI judge to check for policy violations.
type MetadataComplianceRule struct {
	judge *ai.Judge
}

// NewMetadataComplianceRule creates the rule.
func NewMetadataComplianceRule(judge *ai.Judge) *MetadataComplianceRule {
	return &MetadataComplianceRule{judge: judge}
}

// Name returns the rule identifier.
func (r *MetadataComplianceRule) Name() string { return "metadata_compliance" }

// Evaluate asks the AI to review metadata.
func (r *MetadataComplianceRule) Evaluate(inp *Input) *Result {
	specJSON, _ := json.Marshal(inp.Spec)
	res, err := r.judge.Review(
		"You are a compliance reviewer. Check for disallowed content and policy violations.",
		"Review this metadata. Return JSON {'verdict':'approved|rejected|needs_review','comment':'...'}.\nMetadata: "+string(specJSON),
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
