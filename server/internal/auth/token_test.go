package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenAuthDisabled(t *testing.T) {
	a := NewTokenAuth("")
	if a.Enabled() {
		t.Fatalf("empty token should disable auth")
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := a.Verify(r); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTokenAuthVerifies(t *testing.T) {
	a := NewTokenAuth("s3cr3t")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := a.Verify(r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	r.Header.Set("Authorization", "Bearer s3cr3t")
	if err := a.Verify(r); err != nil {
		t.Fatalf("want pass, got %v", err)
	}
	r.Header.Set("Authorization", "Bearer wrong")
	if err := a.Verify(r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized for wrong token, got %v", err)
	}
}
