package rules

import (
	"fmt"
	"strings"
)

// DangerousPermissionsRule flags risky permissions or code patterns.
type DangerousPermissionsRule struct{}

var dangerousSet = map[string]bool{
	"shell-exec": true, "exec": true, "eval": true,
	"child_process": true, "os.system": true, "subprocess": true,
}

// Name returns the rule identifier.
func (r *DangerousPermissionsRule) Name() string { return "dangerous_permissions" }

// Evaluate scans spec.permissions and file contents.
func (r *DangerousPermissionsRule) Evaluate(inp *Input) *Result {
	var found []string

	if perms, ok := inp.Spec["permissions"].([]interface{}); ok {
		for _, p := range perms {
			if s, ok := p.(string); ok {
				lower := strings.ToLower(s)
				for d := range dangerousSet {
					if strings.Contains(lower, d) {
						found = append(found, s)
						break
					}
				}
			}
		}
	}

	for fname, content := range inp.Files {
		text := strings.ToLower(string(content))
		for d := range dangerousSet {
			if strings.Contains(text, d) {
				found = append(found, fmt.Sprintf("%s contains %q", fname, d))
			}
		}
	}

	if len(found) > 0 {
		return &Result{
			Rule:    r.Name(),
			Passed:  false,
			Block:   true,
			Comment: fmt.Sprintf("dangerous: %v", found),
		}
	}
	return &Result{Rule: r.Name(), Passed: true, Comment: "no dangerous permissions"}
}
