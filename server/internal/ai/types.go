// Package ai implements a thin HTTP client to a remote AI judge service used
// by review rules (skill_code_safety, prompt_quality, etc.). The protocol is
// intentionally minimal so enterprise operators can plug in their own model
// gateway without forking the server.
package ai

// Verdict is the qualitative judgment returned by an AI judge.
type Verdict string

const (
	VerdictPass Verdict = "pass"
	VerdictWarn Verdict = "warn"
	VerdictFail Verdict = "fail"
)

// Valid reports whether the verdict is one of the accepted values.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictPass, VerdictWarn, VerdictFail:
		return true
	}
	return false
}

// Request is the wire-format request sent to the AI judge endpoint.
type Request struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Context map[string]any `json:"context,omitempty"`
}

// Response is the wire-format response from the AI judge endpoint.
type Response struct {
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason"`
}
