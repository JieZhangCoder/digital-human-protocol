package rules

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderMarkdown produces a human-friendly report suitable for PR comments.
func RenderMarkdown(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# DHP Review Report\n\n")
	if r.Slug != "" {
		fmt.Fprintf(&b, "- **Slug:** `%s`\n", r.Slug)
	}
	if r.Version != "" {
		fmt.Fprintf(&b, "- **Version:** %s\n", r.Version)
	}
	if r.Type != "" {
		fmt.Fprintf(&b, "- **Type:** %s\n", r.Type)
	}
	fmt.Fprintf(&b, "- **Overall:** %s\n", strings.ToUpper(r.Overall.String()))
	if r.RequiresHuman {
		fmt.Fprintf(&b, "- **Requires human review:** yes\n")
	}
	fmt.Fprintf(&b, "\n## Rules\n\n")
	fmt.Fprintf(&b, "| Rule | Verdict | Notes |\n|---|---|---|\n")
	for _, v := range r.Verdicts {
		notes := strings.ReplaceAll(v.Message, "|", `\|`)
		if v.Skipped {
			notes = "_skipped_ — " + notes
		}
		if len(v.Details) > 0 {
			details := strings.ReplaceAll(strings.Join(v.Details, "; "), "|", `\|`)
			notes = notes + " — " + details
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", v.Rule, strings.ToUpper(v.Severity.String()), notes)
	}
	return b.String()
}

// RenderJSON marshals the report as indented JSON.
func RenderJSON(r *Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
