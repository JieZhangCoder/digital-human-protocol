package rules

import (
	"encoding/json"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
)

// SkillCodeSafetyRule uses the AI judge to review skill code.
type SkillCodeSafetyRule struct {
	judge *ai.Judge
}

// NewSkillCodeSafetyRule creates the rule.
func NewSkillCodeSafetyRule(judge *ai.Judge) *SkillCodeSafetyRule {
	return &SkillCodeSafetyRule{judge: judge}
}

// Name returns the rule identifier.
func (r *SkillCodeSafetyRule) Name() string { return "skill_code_safety" }

// Evaluate asks the AI to review files for security issues.
func (r *SkillCodeSafetyRule) Evaluate(inp *Input) *Result {
	files := make(map[string]string)
	for k, v := range inp.Files {
		files[k] = string(v)
	}
	filesJSON, _ := json.Marshal(files)
	res, err := r.judge.Review(
		"You are a security reviewer for AI skill code. Be strict.",
		"Review these skill files for security issues. Return JSON {'verdict':'approved|rejected|needs_review','comment':'...'}.\nFiles: "+string(filesJSON),
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
