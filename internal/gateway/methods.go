package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lhdbsbz/aido/internal/agent"
	llmpkg "github.com/lhdbsbz/aido/internal/llm"
)

func (s *Server) registerMethods() {
	s.methods["inbound.message"] = s.handleInboundMessage
	s.methods["chat.send"] = s.handleChatSend
	s.methods["chat.history"] = s.handleChatHistory
	s.methods["sessions.list"] = s.handleSessionsList
	s.methods["health"] = s.handleHealthMethod
}

// handleInboundMessage processes messages from bridges.
func (s *Server) handleInboundMessage(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p InboundMessageParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// Convert attachments to agent format
	var images []agent.ImageAttachment
	for _, a := range p.Attach {
		if a.Type == "image" {
			images = append(images, agent.ImageAttachment{
				URL:    a.URL,
				Base64: a.Base64,
				MIME:   a.MIME,
			})
		}
	}

	// Create event sink that broadcasts to all connected clients
	eventSink := func(evt agent.Event) {
		s.Conns.Broadcast("agent", evt)
	}

	result, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		AgentID:   p.AgentID,
		Channel:   p.Channel,
		ChatID:    p.ChatID,
		SenderID:  p.SenderID,
		Text:      p.Text,
		Images:    images,
		MessageID: p.MessageID,
	}, eventSink)
	if err != nil {
		return nil, err
	}

	// Send response back to the originating bridge channel
	s.Conns.BroadcastToChannel(p.Channel, "outbound.message", map[string]any{
		"chatId": p.ChatID,
		"text":   result,
	})

	return map[string]any{"text": result}, nil
}

// handleChatSend processes direct messages from clients.
func (s *Server) handleChatSend(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p ChatSendParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	var images []agent.ImageAttachment
	for _, a := range p.Attach {
		if a.Type == "image" {
			images = append(images, agent.ImageAttachment{
				URL:    a.URL,
				Base64: a.Base64,
				MIME:   a.MIME,
			})
		}
	}

	eventSink := func(evt agent.Event) {
		conn.Send(EventFrame("agent", evt.Seq, evt))
	}

	chatID := p.SessionKey
	if chatID == "" {
		chatID = conn.ID
	}

	result, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		AgentID: p.AgentID,
		Channel: "webchat",
		ChatID:  chatID,
		Text:    p.Text,
		Images:  images,
	}, eventSink)
	if err != nil {
		return nil, err
	}

	return map[string]any{"text": result}, nil
}

// handleChatHistory returns the conversation history for a session.
func (s *Server) handleChatHistory(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p struct {
		SessionKey string `json:"sessionKey"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	entry := s.Router.Store().Get(p.SessionKey)
	if entry == nil {
		return map[string]any{"messages": []any{}}, nil
	}

	transcriptPath := s.Router.Store().TranscriptPath(p.SessionKey)
	_ = transcriptPath

	// Load messages from transcript
	messages, err := loadTranscriptMessages(s.Router.Store().TranscriptPath(p.SessionKey))
	if err != nil {
		return nil, err
	}

	simplified := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		m := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if len(msg.ToolCalls) > 0 {
			m["toolCalls"] = msg.ToolCalls
		}
		simplified = append(simplified, m)
	}

	return map[string]any{"messages": simplified}, nil
}

func loadTranscriptMessages(path string) ([]llmpkg.Message, error) {
	// Reuse session.Transcript for loading
	// For now, inline a simple implementation
	t := &transcriptReader{path: path}
	return t.load()
}

// handleSessionsList returns all sessions.
func (s *Server) handleSessionsList(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	entries := s.Router.Store().List()
	sessions := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		sessions = append(sessions, map[string]any{
			"sessionKey":   e.SessionKey,
			"agentId":      e.AgentID,
			"createdAt":    e.CreatedAt,
			"updatedAt":    e.UpdatedAt,
			"inputTokens":  e.InputTokens,
			"outputTokens": e.OutputTokens,
			"compactions":  e.Compactions,
		})
	}
	return map[string]any{"sessions": sessions}, nil
}

// handleHealthMethod returns health info via WebSocket.
func (s *Server) handleHealthMethod(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	return map[string]any{
		"status":  "ok",
		"bridges": s.Conns.ListBridges(),
		"clients": s.Conns.ClientCount(),
	}, nil
}
