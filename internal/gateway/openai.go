package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/config"
)

// RegisterOpenAICompat registers OpenAI-compatible /v1/chat/completions on the Gin engine.
func (s *Server) RegisterOpenAICompat(engine *gin.Engine) {
	engine.POST("/v1/chat/completions", s.ginOpenAIChatCompletions)
}

// openAIContentPart is one item in messages[].content when content is an array.
type openAIContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

func parseOpenAIUserMessage(content json.RawMessage) (text string, attachments []agent.Attachment, err error) {
	if len(content) == 0 {
		return "", nil, nil
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s, nil, nil
	}
	var parts []openAIContentPart
	if err := json.Unmarshal(content, &parts); err != nil {
		return "", nil, fmt.Errorf("content must be string or array: %w", err)
	}
	var texts []string
	for i, p := range parts {
		switch strings.ToLower(p.Type) {
		case "text":
			texts = append(texts, p.Text)
		case "image_url":
			if p.ImageURL == nil || p.ImageURL.URL == "" {
				return "", nil, fmt.Errorf("content part %d: image_url missing url", i+1)
			}
			url := strings.TrimSpace(p.ImageURL.URL)
			if len(attachments) >= maxAttachmentsPerMessage {
				return "", nil, fmt.Errorf("too many image parts: max %d", maxAttachmentsPerMessage)
			}
			if strings.HasPrefix(url, "data:") {
				base64Str, mime, err := parseDataURL(url)
				if err != nil {
					return "", nil, fmt.Errorf("content part %d: %w", i+1, err)
				}
				decoded, err := base64.StdEncoding.DecodeString(base64Str)
				if err != nil {
					return "", nil, fmt.Errorf("content part %d: invalid base64 in data URL: %w", i+1, err)
				}
				if len(decoded) > maxAttachmentBase64Bytes {
					return "", nil, fmt.Errorf("content part %d: image too large (max %d bytes)", i+1, maxAttachmentBase64Bytes)
				}
				attachments = append(attachments, agent.Attachment{Type: "image", Base64: base64Str, MIME: mime})
			} else {
				attachments = append(attachments, agent.Attachment{Type: "image", URL: url})
			}
		default:
			// ignore unknown part types
		}
	}
	return strings.Join(texts, "\n"), attachments, nil
}

func parseDataURL(dataURL string) (base64Str, mime string, err error) {
	const prefix = "data:"
	if !strings.HasPrefix(dataURL, prefix) {
		return "", "", fmt.Errorf("not a data URL")
	}
	rest := dataURL[len(prefix):]
	idx := strings.Index(rest, ";base64,")
	if idx < 0 {
		return "", "", fmt.Errorf("data URL must contain ;base64,")
	}
	mime = strings.TrimSpace(rest[:idx])
	if mime == "" {
		mime = "image/png"
	}
	base64Str = rest[idx+len(";base64,"):]
	return base64Str, mime, nil
}

func (s *Server) ginOpenAIChatCompletions(c *gin.Context) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !s.authenticate(token) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{"message": "invalid token", "type": "auth_error"},
		})
		return
	}

	var req openAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"message": "invalid json", "type": "invalid_request"},
		})
		return
	}

	var userText string
	var userAttachments []agent.Attachment
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			text, att, err := parseOpenAIUserMessage(req.Messages[i].Content)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": gin.H{"message": err.Error(), "type": "invalid_request"},
				})
				return
			}
			userText = text
			userAttachments = att
			break
		}
	}
	if userText == "" && len(userAttachments) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"message": "no user message found", "type": "invalid_request"},
		})
		return
	}

	cfg := config.Get()
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config not loaded"})
		return
	}
	agentID := cfg.Gateway.CurrentAgent
	if agentID == "" {
		agentID = req.Model
	}
	if agentID == "aido" || agentID == "" {
		agentID = "default"
	}
	sessionKey := req.User
	if sessionKey == "" {
		sessionKey = fmt.Sprintf("openai:%s:%d", agentID, time.Now().UnixMilli())
	}

	if req.Stream {
		s.handleOpenAIStream(c, agentID, sessionKey, userText, userAttachments)
	} else {
		s.handleOpenAISync(c, agentID, sessionKey, userText, userAttachments)
	}
}

func (s *Server) handleOpenAISync(c *gin.Context, agentID, sessionKey, userText string, attachments []agent.Attachment) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	result, _, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		AgentID:     agentID,
		Channel:     "openai",
		ChatID:      sessionKey,
		Text:        userText,
		Attachments: attachments,
	}, nil)
	if err != nil {
		slog.Error("openai compat error", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"message": err.Error(), "type": "server_error"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   agentID,
		"choices": []gin.H{
			{
				"index":         0,
				"message":       gin.H{"role": "assistant", "content": result},
				"finish_reason": "stop",
			},
		},
	})
}

func (s *Server) handleOpenAIStream(c *gin.Context, agentID, sessionKey, userText string, attachments []agent.Attachment) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"message": "streaming not supported", "type": "server_error"},
		})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	flusher.Flush()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	completionID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli())

	eventSink := func(evt agent.Event) {
		if evt.Type == agent.EventTypeTextDelta && evt.Text != "" {
			chunk := map[string]any{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   agentID,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{"content": evt.Text},
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			flusher.Flush()
		}
		if evt.Type == agent.EventTypeDone {
			doneChunk := map[string]any{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   agentID,
				"choices": []map[string]any{
					{
						"index":         0,
						"delta":         map[string]any{},
						"finish_reason": "stop",
					},
				},
			}
			data, _ := json.Marshal(doneChunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			flusher.Flush()
		}
	}

	_, _, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		AgentID:     agentID,
		Channel:     "openai",
		ChatID:      sessionKey,
		Text:        userText,
		Attachments: attachments,
	}, eventSink)
	if err != nil {
		slog.Error("openai stream error", "error", err)
		errChunk := map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "server_error"},
		}
		data, _ := json.Marshal(errChunk)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// OpenAI API types

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	User     string          `json:"user,omitempty"`
}

type openAIMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}
