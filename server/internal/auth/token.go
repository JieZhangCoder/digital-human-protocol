// Package auth implements the registry's bearer-token authentication.
//
// The token is provisioned via config.yaml and is compared with
// crypto/subtle.ConstantTimeCompare to avoid leaking byte-position info.
// Rotating the token requires restarting the registry process — this is
// intentional and documented in the README; in-place reloading would
// significantly complicate the trust model.
package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// ErrUnauthorized is returned when the bearer token check fails.
var ErrUnauthorized = errors.New("auth: unauthorized")

// TokenAuth verifies Authorization: Bearer <token> headers against a single
// configured token.
type TokenAuth struct {
	expected []byte
}

// NewTokenAuth builds a TokenAuth. An empty token disables auth — useful for
// the internal loopback review endpoint where the network ACL is the boundary.
func NewTokenAuth(token string) *TokenAuth {
	return &TokenAuth{expected: []byte(token)}
}

// Enabled reports whether a non-empty token was provided.
func (t *TokenAuth) Enabled() bool { return len(t.expected) > 0 }

// Verify extracts the bearer token from r and returns nil if it matches.
func (t *TokenAuth) Verify(r *http.Request) error {
	if !t.Enabled() {
		return nil
	}
	got := extractBearer(r.Header.Get("Authorization"))
	if got == "" {
		return ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(got), t.expected) != 1 {
		return ErrUnauthorized
	}
	return nil
}

// Middleware wraps next so the handler only runs after a successful token check.
func (t *TokenAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := t.Verify(r); err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="dhp-registry"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractBearer(h string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
