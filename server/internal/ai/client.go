package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrDisabled is returned by Judge when no endpoint is configured. Callers
// should treat this as "AI judge is intentionally disabled" rather than a
// transport error.
var ErrDisabled = errors.New("ai judge disabled (no endpoint configured)")

// Client posts to a remote AI judge endpoint. It is safe for concurrent use.
//
// Protocol: OpenAI-compatible chat.completions. The client sends a system
// message that constrains the model to a strict JSON response shape, then
// extracts the verdict from one of three response forms:
//
//  1. Server-sent events (`text/event-stream`) — delta chunks accumulated.
//  2. Non-streaming chat.completions JSON — content read from
//     `choices[0].message.content`.
//  3. Legacy `{verdict, reason}` JSON — used directly without unwrapping.
//
// The model's content is unwrapped from markdown code fences before being
// JSON-decoded. This makes the client robust against models that return
// ` ```json ... ``` ` wrappers (common with reasoning models).
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

const judgeSystemPrompt = `You are a strict registry reviewer for the Halo Digital Human Protocol.
You inspect candidate apps and return a verdict in JSON.

Respond ONLY with a JSON object matching this schema:

  {"verdict": "pass" | "warn" | "fail", "reason": "<one sentence>"}

Do not add prose, markdown headings, or commentary outside the JSON.
"pass" = no concerns; "warn" = human should look but not blocking;
"fail" = block publication.`

// Judge issues a single judgement request. Network or protocol failures are
// returned as errors so callers can decide whether to fail-closed (block) or
// fail-open (warn). The caller's context controls cancellation.
//
// `prompt` is the rule-specific instruction; `contextData` is JSON-encoded
// and appended as additional context for the model.
func (c *Client) Judge(ctx context.Context, prompt string, contextData map[string]any) (*Response, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}

	userPrompt, err := buildUserPrompt(prompt, contextData)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: judgeSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		// stream:true is the most permissive default — providers that
		// don't support streaming return a normal JSON body, which we
		// detect via Content-Type and parse as chat.completions.
		Stream: true,
	})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai judge transport: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("ai judge status %d: %s", resp.StatusCode, string(raw))
	}

	content, direct, err := readJudgeResponse(resp)
	if err != nil {
		return nil, err
	}

	// Case 3: provider returned the legacy `{verdict, reason}` shape directly.
	if direct != nil {
		if !direct.Verdict.Valid() {
			return nil, fmt.Errorf("invalid verdict %q", direct.Verdict)
		}
		return direct, nil
	}

	// Cases 1 + 2: parse the assistant's message content as our JSON response.
	parsed, err := parseVerdictContent(content)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

// chatRequest / chatMessage match the OpenAI chat.completions request shape.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// buildUserPrompt encodes the rule prompt plus context data into a single
// user message. Context data is appended as a fenced JSON block so the
// model sees structured input without us having to invent another role.
func buildUserPrompt(prompt string, contextData map[string]any) (string, error) {
	var b strings.Builder
	b.WriteString(prompt)
	if len(contextData) > 0 {
		raw, err := json.MarshalIndent(contextData, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode context: %w", err)
		}
		b.WriteString("\n\nContext:\n```json\n")
		b.Write(raw)
		b.WriteString("\n```")
	}
	return b.String(), nil
}

// readJudgeResponse reads the body and returns either:
//   - (content, nil, nil)  — assistant message text extracted; caller must
//     parse it into a Response.
//   - ("", direct, nil)    — provider returned the legacy `{verdict,reason}`
//     shape directly; no further parsing required.
//   - ("", nil, err)       — IO or protocol failure.
func readJudgeResponse(resp *http.Response) (string, *Response, error) {
	contentType := resp.Header.Get("Content-Type")

	// Buffer a small read so we can peek at the body shape when the
	// provider lies about Content-Type (some Chinese gateways return
	// `application/json` for SSE streams or vice versa).
	br := bufio.NewReader(io.LimitReader(resp.Body, 4<<20))
	peek, _ := br.Peek(8)

	switch {
	case strings.Contains(contentType, "text/event-stream"), len(peek) >= 5 && string(peek[:5]) == "data:":
		text, err := readSSE(br)
		if err != nil {
			return "", nil, err
		}
		return text, nil, nil

	default:
		raw, err := io.ReadAll(br)
		if err != nil {
			return "", nil, fmt.Errorf("read response: %w", err)
		}
		var generic map[string]any
		if err := json.Unmarshal(raw, &generic); err != nil {
			return "", nil, fmt.Errorf("decode response: %w (body=%s)", err, truncate(string(raw), 256))
		}

		// Legacy direct shape: top-level {verdict, reason}.
		if _, ok := generic["verdict"]; ok {
			if _, hasChoices := generic["choices"]; !hasChoices {
				var direct Response
				if err := json.Unmarshal(raw, &direct); err != nil {
					return "", nil, fmt.Errorf("decode legacy verdict: %w", err)
				}
				return "", &direct, nil
			}
		}

		// Standard chat.completions: content lives at choices[0].message.content.
		choices, ok := generic["choices"].([]any)
		if !ok || len(choices) == 0 {
			return "", nil, fmt.Errorf("response missing choices (body=%s)", truncate(string(raw), 256))
		}
		choice, ok := choices[0].(map[string]any)
		if !ok {
			return "", nil, errors.New("choices[0] is not an object")
		}
		msg, ok := choice["message"].(map[string]any)
		if !ok {
			return "", nil, errors.New("choices[0].message is not an object")
		}
		content, _ := msg["content"].(string)
		return content, nil, nil
	}
}

// readSSE consumes an OpenAI-style server-sent event stream, accumulating
// `choices[0].delta.content` chunks until [DONE] or EOF.
func readSSE(r *bufio.Reader) (string, error) {
	var sb strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed SSE event
		}
		if len(chunk.Choices) > 0 {
			sb.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("sse scan: %w", err)
	}
	return sb.String(), nil
}

// parseVerdictContent extracts and decodes the assistant's JSON answer.
// It first peels off a single ```json ... ``` (or plain ``` ... ```)
// fence if present — reasoning-trained models routinely wrap their JSON.
func parseVerdictContent(content string) (*Response, error) {
	clean := unwrapFence(strings.TrimSpace(content))
	var out Response
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return nil, fmt.Errorf("decode verdict: %w (content=%s)", err, truncate(content, 256))
	}
	if !out.Verdict.Valid() {
		return nil, fmt.Errorf("invalid verdict %q", out.Verdict)
	}
	return &out, nil
}

// unwrapFence removes an outer ```json ... ``` (or ``` ... ```) wrapper
// if present, returning the inner payload unchanged otherwise.
func unwrapFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	first := strings.Index(s, "\n")
	if first == -1 {
		return s
	}
	last := strings.LastIndex(s, "```")
	if last <= first {
		return s
	}
	return strings.TrimSpace(s[first+1 : last])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
