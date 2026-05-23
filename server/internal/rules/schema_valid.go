package rules

import (
	"context"
	"errors"

	"github.com/openkursar/digital-human-protocol/server/internal/spec"
)

func init() { Register("schema_valid", checkSchemaValid) }

// checkSchemaValid runs the structural spec validator. Failures block
// publication — a malformed spec means the runtime would refuse to install
// anyway.
func checkSchemaValid(_ context.Context, opts *Options) Verdict {
	if err := spec.Validate(opts.Spec); err != nil {
		var ve *spec.ValidationError
		if errors.As(err, &ve) {
			return Verdict{
				Severity: SeverityFail,
				Message:  "spec failed schema validation",
				Details:  ve.Issues,
			}
		}
		return Verdict{Severity: SeverityFail, Message: err.Error()}
	}
	return Verdict{Severity: SeverityPass, Message: "spec passes schema validation"}
}
