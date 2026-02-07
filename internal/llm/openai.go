package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIClient implements Client for all OpenAI-compatible providers
// (OpenAI, DeepSeek, Groq, Mistral, OpenRouter, local models, etc.)
type OpenAIClient struct {
	HTTPClient *http.Client
}

func NewOpenAIClient() *OpenAIClient {
	return &OpenAIClient{HTTPClient: http.DefaultClient}
}

func (c *OpenAIClient) Chat(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	baseURL := params.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	baseURL = strings.TrimRight(baseURL, "/") + "/v1"

	body := c.buildRequest(params)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+params.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(errBody)}
	}

	ch := make(chan StreamEvent, 32)
	go c.consumeSSE(resp.Body, ch)
	return ch, nil
}

func (c *OpenAIClient) buildRequest(params ChatParams) map[string]any {
	messages := make([]map[string]any, 0, len(params.Messages))

	// System prompt as first message
	if params.System != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": params.System,
		})
	}

	for _, msg := range params.Messages {
		m := map[string]any{"role": msg.Role}

		if msg.Role == RoleTool {
			m["tool_call_id"] = msg.ToolCallID
			m["content"] = msg.Content
		} else if len(msg.ToolCalls) > 0 {
			tcs := make([]map[string]any, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				tcs[i] = map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				}
			}
			m["tool_calls"] = tcs
			if msg.Content != "" {
				m["content"] = msg.Content
			}
		} else if len(msg.Images) > 0 {
			content := []map[string]any{
				{"type": "text", "text": msg.Content},
			}
			for _, img := range msg.Images {
				if img.Base64 != "" {
					mime := img.MIME
					if mime == "" {
						mime = "image/png"
					}
					content = append(content, map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:" + mime + ";base64," + img.Base64,
						},
					})
				} else if img.URL != "" {
					content = append(content, map[string]any{
						"type":      "image_url",
						"image_url": map[string]any{"url": img.URL},
					})
				}
			}
			m["content"] = content
		} else {
			m["content"] = msg.Content
		}

		messages = append(messages, m)
	}

	req := map[string]any{
		"model":    params.Model,
		"messages": messages,
		"stream":   true,
	}

	if len(params.Tools) > 0 {
		tools := make([]map[string]any, len(params.Tools))
		for i, t := range params.Tools {
			tools[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  json.RawMessage(t.Parameters),
				},
			}
		}
		req["tools"] = tools
	}

	return req
}

func (c *OpenAIClient) consumeSSE(body io.ReadCloser, out chan<- StreamEvent) {
	defer close(out)
	defer body.Close()

	for event := range ParseSSE(body) {
		var chunk openAIChunk
		if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
			out <- StreamEvent{Type: "error", Error: fmt.Errorf("parse chunk: %w", err)}
			return
		}

		if len(chunk.Choices) == 0 {
			// Usage-only chunk (some providers send this at the end)
			if chunk.Usage != nil {
				out <- StreamEvent{
					Type:  "usage",
					Usage: &Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens},
				}
			}
			continue
		}

		delta := chunk.Choices[0].Delta
		finishReason := chunk.Choices[0].FinishReason

		if delta.Content != "" {
			out <- StreamEvent{Type: "text_delta", Text: delta.Content}
		}

		for _, tc := range delta.ToolCalls {
			out <- StreamEvent{
				Type:          "tool_call_delta",
				ToolCallIndex: tc.Index,
				ToolCallID:    tc.ID,
				ToolCallName:  tc.Function.Name,
				ToolCallArgs:  tc.Function.Arguments,
			}
		}

		if finishReason != "" {
			if chunk.Usage != nil {
				out <- StreamEvent{
					Type:  "usage",
					Usage: &Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens},
				}
			}
			out <- StreamEvent{Type: "done", Text: finishReason}
			return
		}
	}
}

// OpenAI streaming response types

type openAIChunk struct {
	Choices []openAIChoice `json:"choices"`
	Usage   *openAIUsage   `json:"usage,omitempty"`
}

type openAIChoice struct {
	Delta        openAIDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type openAIDelta struct {
	Content   string             `json:"content"`
	ToolCalls []openAIToolCallDelta `json:"tool_calls"`
}

type openAIToolCallDelta struct {
	Index    int                     `json:"index"`
	ID       string                  `json:"id"`
	Function openAIFunctionCallDelta `json:"function"`
}

type openAIFunctionCallDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// APIError represents an HTTP error from the LLM provider.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("LLM API error (status %d): %s", e.StatusCode, e.Body)
}

// IsRateLimit returns true if this is a rate limit error.
func (e *APIError) IsRateLimit() bool { return e.StatusCode == 429 }

// IsAuth returns true if this is an authentication error.
func (e *APIError) IsAuth() bool { return e.StatusCode == 401 || e.StatusCode == 403 }

// IsContextOverflow returns true if context is too long.
func (e *APIError) IsContextOverflow() bool { return e.StatusCode == 400 }
