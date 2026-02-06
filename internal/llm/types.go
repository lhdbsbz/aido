package llm

import "encoding/json"

// Role constants
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message represents a conversation message.
type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Images     []ImageData `json:"images,omitempty"`
}

type ImageData struct {
	URL    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
	MIME   string `json:"mime,omitempty"`
}

// ToolCall represents an LLM's request to call a tool.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

// ToolDef defines a tool for the LLM.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// StreamEvent represents a single event in a streaming LLM response.
type StreamEvent struct {
	Type string // "text_delta" | "tool_call_delta" | "usage" | "done" | "error"

	// For text_delta
	Text string

	// For tool_call_delta
	ToolCallIndex int
	ToolCallID    string
	ToolCallName  string
	ToolCallArgs  string // partial JSON fragment

	// For usage
	Usage *Usage

	// For error
	Error error
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamResult is the accumulated result after consuming a full stream.
type StreamResult struct {
	Message   Message   // the complete assistant message
	ToolCalls []ToolCall // parsed tool calls (if any)
	Text      string     // final text content
	Usage     *Usage
	StopReason string
}

// Helper constructors

func UserMessage(text string) Message {
	return Message{Role: RoleUser, Content: text}
}

func UserMessageWithImages(text string, images []ImageData) Message {
	return Message{Role: RoleUser, Content: text, Images: images}
}

func AssistantMessage(text string) Message {
	return Message{Role: RoleAssistant, Content: text}
}

func AssistantToolCallMessage(toolCalls []ToolCall) Message {
	return Message{Role: RoleAssistant, ToolCalls: toolCalls}
}

func ToolResultMessage(toolCallID, content string) Message {
	return Message{Role: RoleTool, ToolCallID: toolCallID, Content: content}
}

func SystemMessage(text string) Message {
	return Message{Role: RoleSystem, Content: text}
}
