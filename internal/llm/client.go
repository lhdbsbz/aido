package llm

import "context"

// Client is the unified interface for LLM providers.
type Client interface {
	// Chat sends messages to the LLM and returns a stream of events.
	// The caller must consume the channel until it's closed.
	Chat(ctx context.Context, params ChatParams) (<-chan StreamEvent, error)
}

type ChatParams struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	Messages []Message
	Tools    []ToolDef
	System   string // system prompt (extracted from messages for Anthropic)
}

// ConsumeStream reads all events from a stream and returns the accumulated result.
func ConsumeStream(ctx context.Context, stream <-chan StreamEvent) (*StreamResult, error) {
	result := &StreamResult{}
	var textBuilder []byte
	toolCallArgs := make(map[int]*[]byte) // index → accumulated JSON fragments

	for event := range stream {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		switch event.Type {
		case "text_delta":
			textBuilder = append(textBuilder, event.Text...)

		case "tool_call_delta":
			if _, ok := toolCallArgs[event.ToolCallIndex]; !ok {
				buf := []byte{}
				toolCallArgs[event.ToolCallIndex] = &buf
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					ID:   event.ToolCallID,
					Name: event.ToolCallName,
				})
			}
			*toolCallArgs[event.ToolCallIndex] = append(*toolCallArgs[event.ToolCallIndex], event.ToolCallArgs...)
			// update ID/Name if provided (first chunk usually has them)
			idx := event.ToolCallIndex
			if idx < len(result.ToolCalls) {
				if event.ToolCallID != "" {
					result.ToolCalls[idx].ID = event.ToolCallID
				}
				if event.ToolCallName != "" {
					result.ToolCalls[idx].Name = event.ToolCallName
				}
			}

		case "usage":
			result.Usage = event.Usage

		case "error":
			return nil, event.Error

		case "done":
			result.StopReason = event.Text
		}
	}

	result.Text = string(textBuilder)

	// Fill in accumulated tool call arguments
	for idx, args := range toolCallArgs {
		if idx < len(result.ToolCalls) {
			result.ToolCalls[idx].Arguments = string(*args)
		}
	}

	// Build the complete assistant message
	if len(result.ToolCalls) > 0 {
		result.Message = Message{
			Role:      RoleAssistant,
			Content:   result.Text,
			ToolCalls: result.ToolCalls,
		}
	} else {
		result.Message = Message{
			Role:    RoleAssistant,
			Content: result.Text,
		}
	}

	return result, nil
}
