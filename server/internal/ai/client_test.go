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

// Verifies the OpenAI chat.completions response path: the assistant's
// JSON answer lives inside choices[0].message.content.
func TestClientChatCompletions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"reason\":\"needs review\"}"}}]}`))
	}))
	defer ts.Close()
	c := New(ts.URL, "m", "")
	res, err := c.Judge(context.Background(), "p", nil)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if res.Verdict != VerdictWarn || res.Reason != "needs review" {
		t.Fatalf("bad parse: %+v", res)
	}
}

// Verifies SSE streaming: delta.content chunks are accumulated and the
// final markdown-fenced JSON is unwrapped before decoding.
func TestClientSSEWithMarkdownFence(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			"```json\n",
			`{"verdict":"fail",`,
			`"reason":"unsafe code"}`,
			"\n```",
		}
		for _, c := range chunks {
			body, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"delta": map[string]string{"content": c}},
				},
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(body)
			_, _ = w.Write([]byte("\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer ts.Close()
	c := New(ts.URL, "m", "")
	res, err := c.Judge(context.Background(), "p", nil)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if res.Verdict != VerdictFail || res.Reason != "unsafe code" {
		t.Fatalf("bad SSE parse: %+v", res)
	}
}

// Verifies the unwrapFence helper handles bare and language-tagged fences.
func TestUnwrapFence(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```":  `{"a":1}`,
		"```\n{\"b\":2}\n```":      `{"b":2}`,
		`{"c":3}`:                  `{"c":3}`,
		"```json\n{\"d\":4}\n```\n": `{"d":4}`,
	}
	for in, want := range cases {
		got := unwrapFence(in)
		if got != want {
			t.Errorf("unwrapFence(%q): got %q want %q", in, got, want)
		}
	}
}
