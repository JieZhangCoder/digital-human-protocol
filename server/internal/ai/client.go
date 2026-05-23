package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrDisabled is returned by Judge when no endpoint is configured. Callers
// should treat this as "AI judge is intentionally disabled" rather than a
// transport error.
var ErrDisabled = errors.New("ai judge disabled (no endpoint configured)")

// Client posts to a remote AI judge endpoint. It is safe for concurrent use.
type Client struct {
	endpoint string
	model    string
	apiKey   string
	http     *http.Client
}

// New constructs an AI judge client. An empty endpoint produces a client that
// returns ErrDisabled from Judge — useful for offline / unit testing modes.
func New(endpoint, model, apiKey string) *Client {
	return &Client{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
		http:     &http.Client{Timeout: 60 * time.Second},
	}
}

// WithHTTPClient lets callers (notably tests) inject a custom transport.
func (c *Client) WithHTTPClient(h *http.Client) *Client {
	c.http = h
	return c
}

// Enabled reports whether the client will attempt network calls.
func (c *Client) Enabled() bool { return c.endpoint != "" }

// Judge issues a single judgement request. Network or protocol failures are
// returned as errors so callers can decide whether to fail-closed (block) or
// fail-open (warn). The caller's context controls cancellation.
func (c *Client) Judge(ctx context.Context, prompt string, contextData map[string]any) (*Response, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	body, err := json.Marshal(Request{Model: c.model, Prompt: prompt, Context: contextData})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai judge transport: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ai judge status %d: %s", resp.StatusCode, string(raw))
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w (body=%s)", err, string(raw))
	}
	if !out.Verdict.Valid() {
		return nil, fmt.Errorf("invalid verdict %q", out.Verdict)
	}
	return &out, nil
}
