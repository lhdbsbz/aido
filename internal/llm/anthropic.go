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

const anthropicAPIURL = "https://api.anthropic.com"
const anthropicAPIVersion = "2023-06-01"

// AnthropicClient implements Client for Anthropic's native API.
type AnthropicClient struct {
	HTTPClient *http.Client
}

func NewAnthropicClient() *AnthropicClient {
	return &AnthropicClient{HTTPClient: http.DefaultClient}
}

func (c *AnthropicClient) Chat(ctx context.Context, params ChatParams) (<-chan StreamEvent, error) {
	endpoint := anthropicAPIURL
	if params.BaseURL != "" {
		endpoint = params.BaseURL
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/v1/messages"
	body := c.buildRequest(params)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", params.APIKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

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

func (c *AnthropicClient) buildRequest(params ChatParams) map[string]any {
	messages := make([]map[string]any, 0, len(params.Messages))

	for _, msg := range params.Messages {
		switch msg.Role {
		case RoleSystem:
			// Anthropic handles system via top-level param, skip in messages
			continue
		case RoleUser:
			m := map[string]any{"role": "user"}
			if len(msg.Images) > 0 {
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
							"type": "image",
							"source": map[string]any{
								"type":       "base64",
								"media_type": mime,
								"data":       img.Base64,
							},
						})
					}
				}
				m["content"] = content
			} else {
				m["content"] = msg.Content
			}
			messages = append(messages, m)
		case RoleAssistant:
			m := map[string]any{"role": "assistant"}
			if len(msg.ToolCalls) > 0 {
				content := []map[string]any{}
				if msg.Content != "" {
					content = append(content, map[string]any{
						"type": "text", "text": msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					var input any
					_ = json.Unmarshal([]byte(tc.Arguments), &input)
					content = append(content, map[string]any{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Name,
						"input": input,
					})
				}
				m["content"] = content
			} else {
				m["content"] = msg.Content
			}
			messages = append(messages, m)
		case RoleTool:
			// Anthropic: tool results are user messages with tool_result content
			messages = append(messages, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{
						"type":        "tool_result",
						"tool_use_id": msg.ToolCallID,
						"content":     msg.Content,
					},
				},
			})
		}
	}

	req := map[string]any{
		"model":      params.Model,
		"messages":   messages,
		"max_tokens": 8192,
		"stream":     true,
	}

	if params.System != "" {
		req["system"] = params.System
	}

	if len(params.Tools) > 0 {
		tools := make([]map[string]any, len(params.Tools))
		for i, t := range params.Tools {
			tools[i] = map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": json.RawMessage(t.Parameters),
			}
		}
		req["tools"] = tools
	}

	return req
}

func (c *AnthropicClient) consumeSSE(body io.ReadCloser, out chan<- StreamEvent) {
	defer close(out)
	defer body.Close()

	var currentToolIndex int
	toolIndexMap := make(map[string]int) // content block index → our tool index

	for event := range ParseSSE(body) {
		switch event.Event {
		case "content_block_start":
			var block anthropicContentBlockStart
			if err := json.Unmarshal([]byte(event.Data), &block); err != nil {
				continue
			}
			if block.ContentBlock.Type == "tool_use" {
				toolIndexMap[fmt.Sprintf("%d", block.Index)] = currentToolIndex
				out <- StreamEvent{
					Type:          "tool_call_delta",
					ToolCallIndex: currentToolIndex,
					ToolCallID:    block.ContentBlock.ID,
					ToolCallName:  block.ContentBlock.Name,
				}
				currentToolIndex++
			}

		case "content_block_delta":
			var delta anthropicContentBlockDelta
			if err := json.Unmarshal([]byte(event.Data), &delta); err != nil {
				continue
			}
			switch delta.Delta.Type {
			case "text_delta":
				out <- StreamEvent{Type: "text_delta", Text: delta.Delta.Text}
			case "input_json_delta":
				idx, ok := toolIndexMap[fmt.Sprintf("%d", delta.Index)]
				if !ok {
					idx = 0
				}
				out <- StreamEvent{
					Type:          "tool_call_delta",
					ToolCallIndex: idx,
					ToolCallArgs:  delta.Delta.PartialJSON,
				}
			}

		case "message_delta":
			var md anthropicMessageDelta
			if err := json.Unmarshal([]byte(event.Data), &md); err != nil {
				continue
			}
			if md.Usage.OutputTokens > 0 {
				out <- StreamEvent{
					Type:  "usage",
					Usage: &Usage{OutputTokens: md.Usage.OutputTokens},
				}
			}
			out <- StreamEvent{Type: "done", Text: md.Delta.StopReason}
			return

		case "message_start":
			var ms anthropicMessageStart
			if err := json.Unmarshal([]byte(event.Data), &ms); err != nil {
				continue
			}
			if ms.Message.Usage.InputTokens > 0 {
				out <- StreamEvent{
					Type:  "usage",
					Usage: &Usage{InputTokens: ms.Message.Usage.InputTokens},
				}
			}

		case "error":
			out <- StreamEvent{Type: "error", Error: fmt.Errorf("anthropic stream error: %s", event.Data)}
			return
		}
	}
}

// Anthropic streaming response types

type anthropicContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	} `json:"content_block"`
}

type anthropicContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

type anthropicMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicMessageStart struct {
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}
