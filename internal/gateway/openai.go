package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lhdbsbz/aido/internal/agent"
)

// RegisterOpenAICompat registers OpenAI-compatible /v1/chat/completions on the Gin engine.
func (s *Server) RegisterOpenAICompat(engine *gin.Engine) {
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		s.handleOpenAIChatCompletions(c.Writer, c.Request)
	})
}

func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if !s.authenticate(token) {
		http.Error(w, `{"error":{"message":"invalid token","type":"auth_error"}}`, http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":{"message":"read body failed"}}`, http.StatusBadRequest)
		return
	}

	var req openAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":{"message":"invalid json"}}`, http.StatusBadRequest)
		return
	}

	// Extract user message (last user message)
	var userText string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userText = req.Messages[i].Content
			break
		}
	}
	if userText == "" {
		http.Error(w, `{"error":{"message":"no user message found"}}`, http.StatusBadRequest)
		return
	}

	// Resolve agent
	agentID := req.Model
	if agentID == "aido" || agentID == "" {
		agentID = "default"
	}

	// Session key from "user" field or generate one
	sessionKey := req.User
	if sessionKey == "" {
		sessionKey = fmt.Sprintf("openai:%s:%d", agentID, time.Now().UnixMilli())
	}

	if req.Stream {
		s.handleOpenAIStream(w, r, agentID, sessionKey, userText)
	} else {
		s.handleOpenAISync(w, r, agentID, sessionKey, userText)
	}
}

func (s *Server) handleOpenAISync(w http.ResponseWriter, r *http.Request, agentID, sessionKey, userText string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	result, _, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		AgentID: agentID,
		Channel: "openai",
		ChatID:  sessionKey,
		Text:    userText,
	}, nil)

	if err != nil {
		slog.Error("openai compat error", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "server_error"},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   agentID,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": result},
				"finish_reason": "stop",
			},
		},
	})
}

func (s *Server) handleOpenAIStream(w http.ResponseWriter, r *http.Request, agentID, sessionKey, userText string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
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
			fmt.Fprintf(w, "data: %s\n\n", data)
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
			fmt.Fprintf(w, "data: %s\n\n", data)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}
	}

	_, _, err := s.Router.HandleMessage(ctx, agent.InboundMessage{
		AgentID: agentID,
		Channel: "openai",
		ChatID:  sessionKey,
		Text:    userText,
	}, eventSink)

	if err != nil {
		slog.Error("openai stream error", "error", err)
		errChunk := map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "server_error"},
		}
		data, _ := json.Marshal(errChunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
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
	Role    string `json:"role"`
	Content string `json:"content"`
}
