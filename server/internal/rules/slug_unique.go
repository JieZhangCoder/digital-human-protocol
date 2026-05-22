package rules

import "fmt"

// SlugUniqueRule prevents duplicate slug+version.
type SlugUniqueRule struct{}

// Name returns the rule identifier.
func (r *SlugUniqueRule) Name() string { return "slug_unique" }

// Evaluate checks existing refs for collisions.
func (r *SlugUniqueRule) Evaluate(inp *Input) *Result {
	for _, ref := range inp.Existing {
		if ref.Slug == inp.Slug && ref.Version == inp.Version {
			return &Result{
				Rule:    r.Name(),
				Passed:  false,
				Block:   true,
				Comment: fmt.Sprintf("slug %s@%s already exists", inp.Slug, inp.Version),
			}
		}
	}
	return &Result{Rule: r.Name(), Passed: true, Comment: "slug is unique"}
}
