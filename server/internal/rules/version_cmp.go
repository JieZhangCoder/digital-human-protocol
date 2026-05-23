package rules

import (
	"strconv"
	"strings"
)

// compareVersions performs a loose semver comparison.
//
// It splits on dots, parses leading integer prefixes, and pads to three
// components. Pre-release / build metadata is ignored — adequate for the
// blocking "downgrade detection" use case, where strict ordering rules are
// not required and false positives would frustrate maintainers.
//
// Returns -1, 0, or +1 like strings.Compare semantics.
func compareVersions(a, b string) int {
	pa := splitVersion(a)
	pb := splitVersion(b)
	for i := 0; i < 3; i++ {
		var ai, bi int
		if i < len(pa) {
			ai = pa[i]
		}
		if i < len(pb) {
			bi = pb[i]
		}
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		}
	}
	return 0
}

func splitVersion(v string) []int {
	v = strings.TrimSpace(v)
	// Drop pre-release / build metadata.
	for _, sep := range []string{"-", "+"} {
		if idx := strings.Index(v, sep); idx >= 0 {
			v = v[:idx]
		}
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		// Extract leading digits.
		end := 0
		for end < len(p) && p[end] >= '0' && p[end] <= '9' {
			end++
		}
		if end == 0 {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(p[:end])
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}
