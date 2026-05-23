package rules

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func init() { Register("dangerous_permissions", checkDangerousPermissions) }

// dangerousPermissions enumerates the permission identifiers that require an
// explicit allowlist entry or human review. The list is conservative — adding
// here only requires updating allowlist entries in deployments that legitimately
// need them.
var dangerousPermissions = map[string]string{
	"shell-exec":                  "executes arbitrary shell commands on the host",
	"filesystem-write-arbitrary":  "writes to arbitrary filesystem paths",
	"filesystem-read-arbitrary":   "reads arbitrary filesystem paths (incl. user secrets)",
	"network-outbound-unrestricted": "performs unrestricted outbound network access",
	"process-spawn":               "spawns arbitrary child processes",
	"env-read":                    "reads environment variables (potential secrets exfiltration)",
}

// checkDangerousPermissions blocks publishes that request flagged permissions
// unless they are explicitly allowlisted in config.rules.allowlist.
func checkDangerousPermissions(_ context.Context, opts *Options) Verdict {
	allow := map[string]bool{}
	if opts.Config != nil {
		for _, p := range opts.Config.Rules.Allowlist.DangerousPermissions {
			allow[p] = true
		}
	}
	var flagged []string
	for _, p := range opts.Spec.Permissions {
		if reason, dangerous := dangerousPermissions[p]; dangerous && !allow[p] {
			flagged = append(flagged, fmt.Sprintf("%s: %s", p, reason))
		}
	}
	sort.Strings(flagged)
	if len(flagged) == 0 {
		return Verdict{Severity: SeverityPass, Message: "no dangerous permissions requested"}
	}
	return Verdict{
		Severity:      SeverityFail,
		Message:       "spec requests dangerous permissions not on the allowlist",
		Details:       flagged,
		RequiresHuman: true,
	}
}

// PermissionList exposes the dangerous permission set for documentation /
// tooling. Sorted for deterministic ordering.
func PermissionList() []string {
	out := make([]string, 0, len(dangerousPermissions))
	for k := range dangerousPermissions {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// trimSpaceLower is a tiny helper used by other rules for case-insensitive
// matching of keywords. Kept here to avoid a separate util file.
func trimSpaceLower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
