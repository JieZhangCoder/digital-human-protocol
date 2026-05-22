package rules

import "fmt"

// SchemaValidRule checks required fields.
type SchemaValidRule struct{}

// Name returns the rule identifier.
func (r *SchemaValidRule) Name() string { return "schema_valid" }

// Evaluate ensures slug, version, name and type are present.
func (r *SchemaValidRule) Evaluate(inp *Input) *Result {
	var errs []string
	if inp.Slug == "" {
		errs = append(errs, "slug is required")
	}
	if inp.Version == "" {
		errs = append(errs, "version is required")
	}
	if name, _ := inp.Spec["name"].(string); name == "" {
		errs = append(errs, "spec.name is required")
	}
	if typ, _ := inp.Spec["type"].(string); typ == "" {
		errs = append(errs, "spec.type is required")
	}
	if len(errs) > 0 {
		msg := ""
		for i, e := range errs {
			if i > 0 {
				msg += "; "
			}
			msg += e
		}
		return &Result{Rule: r.Name(), Passed: false, Block: true, Comment: msg}
	}
	return &Result{Rule: r.Name(), Passed: true, Comment: "schema valid"}
}

func init() {
	_ = fmt.Sprintf("") // silence unused import if fmt becomes unused
}
