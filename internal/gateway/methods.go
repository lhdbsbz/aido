package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/config"
	llmpkg "github.com/lhdbsbz/aido/internal/llm"
)

func (s *Server) handleMessageSend(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p MessageSendParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Channel == "" || p.Text == "" {
		return nil, fmt.Errorf("channel and text required")
	}
	if p.ChannelChatID == "" {
		p.ChannelChatID = "main"
	}

	channel, channelChatId := p.Channel, p.ChannelChatID
	var images []agent.ImageAttachment
	for _, a := range p.Attachments {
		if a.Type == "image" {
			images = append(images, agent.ImageAttachment{URL: a.URL, Base64: a.Base64, MIME: a.MIME})
		}
	}

	s.Conns.BroadcastToRole(RoleClient, "user_message", map[string]any{
		"channel":        channel,
		"channelChatId": channelChatId,
		"text":           p.Text,
	})

	eventSink := func(evt agent.Event) {
		payload := agentEventPayload(evt, channel, channelChatId)
		s.Conns.BroadcastToRole(RoleClient, "agent", payload)
		s.Conns.BroadcastToChannel(channel, "agent", payload)
	}

	result, toolSteps, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		Channel:   channel,
		ChatID:    channelChatId,
		SenderID:  p.SenderID,
		Text:      p.Text,
		Images:    images,
		MessageID: p.MessageID,
	}, eventSink)
	if err != nil {
		return nil, err
	}

	s.Conns.BroadcastToChannel(channel, "outbound.message", map[string]any{
		"channel":        channel,
		"channelChatId": channelChatId,
		"text":           result,
	})

	out := map[string]any{"text": result}
	if len(toolSteps) > 0 {
		out["toolSteps"] = toolSteps
	}
	return out, nil
}

func agentEventPayload(evt agent.Event, channel, channelChatId string) map[string]any {
	m := map[string]any{
		"type":      evt.Type,
		"runId":     evt.RunID,
		"seq":       evt.Seq,
		"timestamp": evt.Timestamp,
		"channel":   channel,
		"channelChatId": channelChatId,
	}
	if evt.Text != "" {
		m["text"] = evt.Text
	}
	if evt.ToolName != "" {
		m["toolName"] = evt.ToolName
		m["toolParams"] = evt.ToolParams
		m["toolResult"] = evt.ToolResult
	}
	if evt.Error != "" {
		m["error"] = evt.Error
	}
	if evt.TotalTokensIn > 0 || evt.TotalTokensOut > 0 {
		m["totalTokensIn"] = evt.TotalTokensIn
		m["totalTokensOut"] = evt.TotalTokensOut
	}
	if evt.Iterations > 0 {
		m["iterations"] = evt.Iterations
	}
	return m
}

func (s *Server) getChatHistory(ctx context.Context, channel, channelChatId string) (any, error) {
	storageKey := SessionKey(channel, channelChatId)
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
		m := map[string]any{"role": msg.Role, "content": msg.Content}
		if len(msg.ToolCalls) > 0 {
			m["toolCalls"] = msg.ToolCalls
		}
		simplified = append(simplified, m)
	}
	return map[string]any{"messages": simplified}, nil
}

func (s *Server) handleChatHistory(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p struct {
		Channel        string `json:"channel"`
		ChannelChatID  string `json:"channelChatId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Channel == "" || p.ChannelChatID == "" {
		return nil, fmt.Errorf("channel and channelChatId required")
	}
	return s.getChatHistory(ctx, p.Channel, p.ChannelChatID)
}

func loadTranscriptMessages(path string) ([]llmpkg.Message, error) {
	t := &transcriptReader{path: path}
	return t.load()
}

func (s *Server) handleSessionsList(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	entries := s.Router.Store().List()
	sessions := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		channel, channelChatId := parseChannelChatId(e.SessionKey)
		sessions = append(sessions, map[string]any{
			"channel":        channel,
			"channelChatId": channelChatId,
			"createdAt":     e.CreatedAt,
			"updatedAt":     e.UpdatedAt,
			"inputTokens":   e.InputTokens,
			"outputTokens":  e.OutputTokens,
			"compactions":   e.Compactions,
		})
	}
	return map[string]any{"sessions": sessions}, nil
}

func parseChannelChatId(sessionKey string) (channel, channelChatId string) {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) == 3 {
		return parts[1], parts[2]
	}
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "main"
	}
	return "direct", "main"
}

func (s *Server) handleHealthMethod(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	return map[string]any{
		"status":  "ok",
		"bridges": s.Conns.ListBridges(),
		"clients": s.Conns.ClientCount(),
	}, nil
}

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
			"locale":       cfg.Gateway.Locale,
			"auth":         map[string]any{"token": cfg.Gateway.Auth.Token},
		},
		"agents": cfg.Agents,
		"tools":  cfg.Tools,
	}
	providers := make(map[string]any)
	for k, p := range cfg.Providers {
		providers[k] = map[string]any{"apiKey": p.APIKey, "baseURL": p.BaseURL, "type": p.Type}
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
