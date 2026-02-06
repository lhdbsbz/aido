package message

// InboundMessage is the normalized message format from any source.
type InboundMessage struct {
	Channel    string       `json:"channel"`
	ChatID     string       `json:"chatId"`
	SenderID   string       `json:"senderId"`
	Text       string       `json:"text"`
	MessageID  string       `json:"messageId,omitempty"`
	AgentID    string       `json:"agentId,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type Attachment struct {
	Type   string `json:"type"` // "image" | "audio" | "video" | "file"
	URL    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
	MIME   string `json:"mime,omitempty"`
	Name   string `json:"name,omitempty"`
}

// OutboundMessage is sent back to a channel via bridge.
type OutboundMessage struct {
	Channel string `json:"channel"`
	ChatID  string `json:"chatId"`
	Text    string `json:"text"`
	ReplyTo string `json:"replyTo,omitempty"`
}
