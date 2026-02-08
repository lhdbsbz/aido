package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/config"
	llmpkg "github.com/lhdbsbz/aido/internal/llm"
)

const (
	maxAttachmentsPerMessage = 20
	maxAttachmentBase64Bytes  = 15 * 1024 * 1024 // 15MB
)

var allowedAttachmentTypes = map[string]bool{"image": true, "audio": true, "video": true, "file": true}

func validateAndConvertAttachments(in []AttachmentParam) ([]agent.Attachment, error) {
	if len(in) > maxAttachmentsPerMessage {
		return nil, fmt.Errorf("too many attachments: max %d", maxAttachmentsPerMessage)
	}
	out := make([]agent.Attachment, 0, len(in))
	for i, a := range in {
		typ := strings.TrimSpace(strings.ToLower(a.Type))
		if typ == "" {
			return nil, fmt.Errorf("attachment %d: type required", i+1)
		}
		if !allowedAttachmentTypes[typ] {
			return nil, fmt.Errorf("attachment %d: invalid type %q (allowed: image, audio, video, file)", i+1, a.Type)
		}
		hasURL := strings.TrimSpace(a.URL) != ""
		hasBase64 := strings.TrimSpace(a.Base64) != ""
		if !hasURL && !hasBase64 {
			return nil, fmt.Errorf("attachment %d: url or base64 required", i+1)
		}
		if hasURL && hasBase64 {
			return nil, fmt.Errorf("attachment %d: provide url or base64, not both", i+1)
		}
		if hasBase64 {
			decoded, err := base64.StdEncoding.DecodeString(a.Base64)
			if err != nil {
				return nil, fmt.Errorf("attachment %d: invalid base64: %w", i+1, err)
			}
			if len(decoded) > maxAttachmentBase64Bytes {
				return nil, fmt.Errorf("attachment %d: base64 too large (max %d bytes)", i+1, maxAttachmentBase64Bytes)
			}
		}
		out = append(out, agent.Attachment{Type: typ, URL: strings.TrimSpace(a.URL), Base64: a.Base64, MIME: strings.TrimSpace(a.MIME)})
	}
	return out, nil
}

func (s *Server) handleMessageSend(ctx context.Context, conn *Conn, params json.RawMessage) (any, error) {
	var p MessageSendParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Channel == "" {
		return nil, fmt.Errorf("channel required")
	}
	if p.Text == "" && len(p.Attachments) == 0 {
		return nil, fmt.Errorf("text or at least one attachment required")
	}
	if p.ChannelChatID == "" {
		p.ChannelChatID = "main"
	}

	attachments, err := validateAndConvertAttachments(p.Attachments)
	if err != nil {
		return nil, err
	}

	channel, channelChatId := p.Channel, p.ChannelChatID

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
		Channel:     channel,
		ChatID:      channelChatId,
		SenderID:    p.SenderID,
		Text:        p.Text,
		Attachments: attachments,
		MessageID:   p.MessageID,
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

// parseChannelChatId splits session key "channel:channelChatId" (channelChatId may contain ":").
func parseChannelChatId(sessionKey string) (channel, channelChatId string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return "direct", "main"
	}
	idx := strings.Index(sessionKey, ":")
	if idx < 0 {
		return sessionKey, "main"
	}
	return sessionKey[:idx], sessionKey[idx+1:]
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
			"locale":       cfg.Gateway.Locale,
			"auth":         map[string]any{"token": cfg.Gateway.Auth.Token},
		},
		"agents":   cfg.Agents,
		"tools":    cfg.Tools,
		"bridges":  cfg.Bridges,
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
		return map[string]any{"configPath": config.Path(), "gateway": map[string]any{}, "agents": map[string]any{}, "providers": map[string]any{}, "tools": map[string]any{}, "bridges": map[string]any{"instances": []any{}}}
	}
	return configForUIFromCfg(cfg)
}
