package rules

import (
	"context"
	"errors"
	"fmt"

	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

func init() { Register("slug_unique", checkSlugUnique) }

// checkSlugUnique queries the configured registry for the slug:
//   - not found             → pass (new app)
//   - found, same version   → warn (re-publish overwrites the artifact)
//   - found, lower version  → pass (legitimate version bump)
//   - found, higher version → fail (cannot publish a downgrade)
//   - registry unreachable  → error (caller decides whether to fail closed)
func checkSlugUnique(ctx context.Context, opts *Options) Verdict {
	slug := opts.Spec.Slug()
	if slug == "" {
		return Verdict{Severity: SeverityPass, Message: "no store.slug declared; skipping uniqueness check"}
	}
	if opts.Registry == nil {
		return Verdict{Severity: SeverityWarn, Message: "no registry configured; skipping uniqueness check"}
	}
	entry, err := opts.Registry.LookupSlug(ctx, slug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return Verdict{Severity: SeverityPass, Message: fmt.Sprintf("slug %q is unused", slug)}
		}
		return Verdict{Severity: SeverityError, Message: fmt.Sprintf("registry lookup failed: %v", err)}
	}
	if entry == nil {
		return Verdict{Severity: SeverityPass, Message: fmt.Sprintf("slug %q is unused", slug)}
	}
	cmp := compareVersions(opts.Spec.Version, entry.Version)
	switch {
	case cmp == 0:
		return Verdict{
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("slug %q already published at the same version %s", slug, entry.Version),
		}
	case cmp < 0:
		return Verdict{
			Severity: SeverityFail,
			Message:  fmt.Sprintf("slug %q is at v%s; cannot publish lower version v%s", slug, entry.Version, opts.Spec.Version),
		}
	default:
		return Verdict{
			Severity: SeverityPass,
			Message:  fmt.Sprintf("slug %q upgrades from v%s to v%s", slug, entry.Version, opts.Spec.Version),
		}
	}
}
