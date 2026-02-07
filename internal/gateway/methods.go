package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/config"
	llmpkg "github.com/lhdbsbz/aido/internal/llm"
)

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

	result, _, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
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

// handleChatSend processes direct messages from clients (WebSocket).
func (s *Server) handleChatSend(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p ChatSendParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	chatID := p.SessionKey
	if chatID == "" {
		chatID = conn.ID
	}
	eventSink := func(evt agent.Event) {
		conn.Send(EventFrame("agent", evt.Seq, evt))
	}
	return s.runChatSend(ctx, &p, chatID, eventSink)
}

// runChatSend runs the chat send logic (shared by WebSocket and HTTP API).
func (s *Server) runChatSend(ctx context.Context, p *ChatSendParams, chatID string, eventSink agent.EventSink) (any, error) {
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
	chatIDForRouter := WebchatChatID(chatID)
	result, toolSteps, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		AgentID: p.AgentID,
		Channel: "webchat",
		ChatID:  chatIDForRouter,
		Text:    p.Text,
		Images:  images,
	}, eventSink)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"text": result}
	if len(toolSteps) > 0 {
		out["toolSteps"] = toolSteps
	}
	return out, nil
}

// getChatHistory returns the conversation history for a session key (used by HTTP API).
func (s *Server) getChatHistory(ctx context.Context, sessionKey string) (any, error) {
	storageKey := ToStorageKey(sessionKey)
	entry := s.Router.Store().Get(storageKey)
	if entry == nil {
		return map[string]any{"messages": []any{}}, nil
	}
	messages, err := loadTranscriptMessages(s.Router.Store().TranscriptPath(storageKey))
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

// handleChatHistory returns the conversation history for a session (WebSocket).
func (s *Server) handleChatHistory(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p struct {
		SessionKey string `json:"sessionKey"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return s.getChatHistory(ctx, p.SessionKey)
}

func loadTranscriptMessages(path string) ([]llmpkg.Message, error) {
	// Reuse session.Transcript for loading
	// For now, inline a simple implementation
	t := &transcriptReader{path: path}
	return t.load()
}

// handleSessionsList returns all sessions. For webchat sessions, sessionKey 转为客户端格式便于前端展示和拉历史。
func (s *Server) handleSessionsList(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	entries := s.Router.Store().List()
	sessions := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		displayKey := e.SessionKey
		if ck := ToClientKey(e.SessionKey); ck != "" {
			displayKey = ck
		}
		sessions = append(sessions, map[string]any{
			"sessionKey":   displayKey,
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

// handleConfigGet returns current config for the management UI (read from file).
func (s *Server) handleConfigGet(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	cfg, err := config.Load(config.Path())
	if err != nil {
		return nil, err
	}
	return configForUIFromCfg(cfg), nil
}

func configForUIFromCfg(cfg *config.Config) map[string]any {
	out := map[string]any{
		"configPath": config.Path(),
		"gateway": map[string]any{
			"port":         cfg.Gateway.Port,
			"currentAgent": cfg.Gateway.CurrentAgent,
			"toolsProfile": cfg.Gateway.ToolsProfile,
			"auth": map[string]any{
				"token": cfg.Gateway.Auth.Token,
			},
		},
		"agents":    cfg.Agents,
		"tools":     cfg.Tools,
	}
	providers := make(map[string]any)
	for k, p := range cfg.Providers {
		providers[k] = map[string]any{
			"apiKey":  p.APIKey,
			"baseURL": p.BaseURL,
			"type":    p.Type,
		}
	}
	out["providers"] = providers
	return out
}

func (s *Server) configForUI() map[string]any {
	cfg := config.Get()
	if cfg == nil {
		return map[string]any{"configPath": config.Path(), "gateway": map[string]any{}, "agents": map[string]any{}, "providers": map[string]any{}, "tools": map[string]any{}}
	}
	return configForUIFromCfg(cfg)
}
