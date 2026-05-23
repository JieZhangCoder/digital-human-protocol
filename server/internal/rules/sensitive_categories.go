package rules

import (
	"context"
	"fmt"
	"strings"
)

func init() { Register("sensitive_categories", checkSensitiveCategories) }

// checkSensitiveCategories flags specs whose name or description hints at
// regulated subject matter (financial advice, medical, election influence...).
// It does not block — it sets requires_human so a maintainer reviews manually.
func checkSensitiveCategories(_ context.Context, opts *Options) Verdict {
	if opts.Config == nil {
		return Verdict{Severity: SeverityPass, Message: "no sensitive keyword list configured"}
	}
	haystack := strings.ToLower(opts.Spec.Name + "\n" + opts.Spec.Description)
	var hits []string
	for _, kw := range opts.Config.Rules.SensitiveKeywords {
		k := trimSpaceLower(kw)
		if k == "" {
			continue
		}
		if strings.Contains(haystack, k) {
			hits = append(hits, kw)
		}
	}
	if len(hits) == 0 {
		return Verdict{Severity: SeverityPass, Message: "no sensitive category keywords detected"}
	}
	return Verdict{
		Severity:      SeverityWarn,
		Message:       fmt.Sprintf("sensitive keywords matched: %s", strings.Join(hits, ", ")),
		RequiresHuman: true,
	}
}
