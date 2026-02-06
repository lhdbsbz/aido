package gateway

import "encoding/json"

// Frame is the universal WebSocket message format.
// Three types: "req" (client→server), "res" (server→client), "event" (server→client push).
type Frame struct {
	Type    string          `json:"type"`              // "req" | "res" | "event"
	ID      string          `json:"id,omitempty"`      // request/response correlation ID
	Method  string          `json:"method,omitempty"`  // for req: method name
	Params  json.RawMessage `json:"params,omitempty"`  // for req: method parameters
	OK      *bool           `json:"ok,omitempty"`      // for res: success flag
	Payload json.RawMessage `json:"payload,omitempty"` // for res: response data
	Error   *ErrorPayload   `json:"error,omitempty"`   // for res: error details
	Event   string          `json:"event,omitempty"`   // for event: event name
	Seq     int             `json:"seq,omitempty"`     // for event: sequence number
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Connection roles
const (
	RoleBridge = "bridge"
	RoleClient = "client"
)

// ConnectParams is sent by the client during handshake.
type ConnectParams struct {
	Role         string   `json:"role"`         // "bridge" | "client"
	Token        string   `json:"token"`        // auth token
	Channel      string   `json:"channel"`      // for bridges: channel name
	Capabilities []string `json:"capabilities"` // for bridges: supported features
}

// InboundMessageParams is sent by bridges to push user messages.
type InboundMessageParams struct {
	AgentID   string              `json:"agentId,omitempty"`
	Channel   string              `json:"channel"`
	ChatID    string              `json:"chatId"`
	SenderID  string              `json:"senderId"`
	Text      string              `json:"text"`
	MessageID string              `json:"messageId,omitempty"`
	Attach    []AttachmentParam   `json:"attachments,omitempty"`
}

type AttachmentParam struct {
	Type   string `json:"type"`   // "image"
	URL    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
	MIME   string `json:"mime,omitempty"`
}

// ChatSendParams is sent by clients to directly chat with an agent.
type ChatSendParams struct {
	AgentID    string            `json:"agentId,omitempty"`
	Text       string            `json:"text"`
	SessionKey string            `json:"sessionKey,omitempty"` // optional: resume specific session
	Attach     []AttachmentParam `json:"attachments,omitempty"`
}

// Helper to create response frames

func ResOK(id string, payload any) Frame {
	data, _ := json.Marshal(payload)
	ok := true
	return Frame{Type: "res", ID: id, OK: &ok, Payload: data}
}

func ResErr(id string, code, message string) Frame {
	ok := false
	return Frame{Type: "res", ID: id, OK: &ok, Error: &ErrorPayload{Code: code, Message: message}}
}

func EventFrame(event string, seq int, payload any) Frame {
	data, _ := json.Marshal(payload)
	return Frame{Type: "event", Event: event, Seq: seq, Payload: data}
}
