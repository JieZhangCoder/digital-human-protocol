package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientDisabled(t *testing.T) {
	c := New("", "", "")
	_, err := c.Judge(context.Background(), "p", nil)
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("want ErrDisabled, got %v", err)
	}
}

func TestClientPass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("missing/incorrect auth header: %q", got)
		}
		json.NewEncoder(w).Encode(Response{Verdict: VerdictPass, Reason: "ok"})
	}))
	defer ts.Close()
	c := New(ts.URL, "test-model", "k")
	res, err := c.Judge(context.Background(), "prompt", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if res.Verdict != VerdictPass || res.Reason != "ok" {
		t.Fatalf("bad response: %+v", res)
	}
}

func TestClientBadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", 500)
	}))
	defer ts.Close()
	c := New(ts.URL, "m", "")
	if _, err := c.Judge(context.Background(), "p", nil); err == nil {
		t.Fatal("want error for 500 status")
	}
}

func TestVerdictValid(t *testing.T) {
	if VerdictPass.Valid() != true || Verdict("nope").Valid() {
		t.Fatal("Verdict.Valid wrong")
	}
}
