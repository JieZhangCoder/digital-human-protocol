package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ReviewResult is the verdict from the AI judge.
type ReviewResult struct {
	Verdict string `json:"verdict"`
	Comment string `json:"comment"`
}

// Judge calls an OpenAI-compatible endpoint.
type Judge struct {
	Endpoint string
	Model    string
	APIKey   string
	client   *http.Client
}

// NewJudge constructs a Judge.
func NewJudge(endpoint, model, apiKey string) *Judge {
	return &Judge{
		Endpoint: endpoint,
		Model:    model,
		APIKey:   apiKey,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// Enabled returns true when an endpoint is configured.
func (j *Judge) Enabled() bool {
	return j.Endpoint != ""
}

// openAIResponse is the standard chat completions response.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Review sends the prompt and parses the JSON response.
// Supports both:
//   1. Direct {verdict, comment} response (legacy wrapper)
//   2. Standard OpenAI chat.completions response
func (j *Judge) Review(systemPrompt, userPrompt string) (*ReviewResult, error) {
	if !j.Enabled() {
		return &ReviewResult{Verdict: "approved", Comment: "AI judge not configured, auto-approved"}, nil
	}

	payload := map[string]interface{}{
		"model": j.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": true,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", j.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if j.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+j.APIKey)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Drain body for error details
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai judge returned %d: %s", resp.StatusCode, string(body))
	}

	// Determine if response is streaming (SSE) or non-streaming JSON.
	contentType := resp.Header.Get("Content-Type")
	var fullContent string

	if strings.Contains(contentType, "text/event-stream") || isSSEResponse(resp.Body) {
		// SSE streaming: concatenate all delta content chunks
		var sb strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
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
				continue
			}
			if len(chunk.Choices) > 0 {
				sb.WriteString(chunk.Choices[0].Delta.Content)
			}
		}
		fullContent = sb.String()
	} else {
		// Non-streaming: standard OpenAI chat.completions response
		var direct map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&direct); err != nil {
			return nil, err
		}

		// If it has a "choices" array, it's standard OpenAI format
		if choices, ok := direct["choices"].([]interface{}); ok && len(choices) > 0 {
			choice, ok := choices[0].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid OpenAI response format")
			}
			msg, ok := choice["message"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid OpenAI response format: no message")
			}
			content, ok := msg["content"].(string)
			if !ok {
				return nil, fmt.Errorf("invalid OpenAI response format: no content")
			}
			fullContent = content
		} else {
			// Legacy direct format: {verdict: "...", comment: "..."}
			verdict := "approved"
			if v, ok := direct["verdict"].(string); ok {
				verdict = v
			}
			comment := ""
			if c, ok := direct["comment"].(string); ok {
				comment = c
			}
			return &ReviewResult{Verdict: verdict, Comment: comment}, nil
		}
	}

	// Parse the accumulated content into a ReviewResult
	extracted := extractJSONFromMarkdown(fullContent)

	var result ReviewResult
	if err := json.Unmarshal([]byte(extracted), &result); err != nil {
		// If content is not valid JSON, treat the whole content as comment
		return &ReviewResult{Verdict: "approved", Comment: fullContent}, nil
	}
	return &result, nil
}

// isSSEResponse peeks at the response body to check if it starts with "data:".
func isSSEResponse(body io.Reader) bool {
	var buf [6]byte
	n, _ := body.Read(buf[:])
	return n >= 5 && string(buf[:5]) == "data:"
}

// extractJSONFromMarkdown tries to extract JSON from markdown code blocks.
// If no code block found, returns the original string.
func extractJSONFromMarkdown(content string) string {
	// Look for ```json ... ``` or ``` ... ```
	start := strings.Index(content, "```json")
	if start == -1 {
		start = strings.Index(content, "```")
	}
	if start == -1 {
		return content
	}
	// Find the end of the opening ```
	codeStart := strings.Index(content[start:], "\n")
	if codeStart == -1 {
		return content
	}
	codeStart += start + 1

	end := strings.Index(content[codeStart:], "```")
	if end == -1 {
		return content
	}
	return strings.TrimSpace(content[codeStart : codeStart+end])
}
