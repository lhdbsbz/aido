package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lhdbsbz/aido/internal/agent"
)

const apiPrefix = "/api"

func (s *Server) apiAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if token == "" {
			token = c.Query("token")
		}
		if !s.authenticate(token) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Next()
	}
}

func (s *Server) registerAPIRoutes(engine *gin.Engine) {
	api := engine.Group(apiPrefix, s.apiAuthMiddleware())
	api.GET("/health", s.ginAPIHealth)
	api.GET("/config", s.ginAPIConfig)
	api.GET("/sessions", s.ginAPISessions)
	api.GET("/chat/history", s.ginAPIChatHistory)
	api.POST("/chat/send", s.ginAPIChatSend)
}

func (s *Server) ginAPIHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"bridges": s.Conns.ListBridges(),
		"clients": s.Conns.ClientCount(),
	})
}

func (s *Server) ginAPIConfig(c *gin.Context) {
	c.JSON(http.StatusOK, s.configForUI())
}

func (s *Server) ginAPISessions(c *gin.Context) {
	result, _ := s.handleSessionsList(c.Request.Context(), nil, nil)
	c.JSON(http.StatusOK, result)
}

func (s *Server) ginAPIChatHistory(c *gin.Context) {
	sessionKey := c.Query("sessionKey")
	if sessionKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionKey required"})
		return
	}
	params, _ := json.Marshal(struct{ SessionKey string }{SessionKey: sessionKey})
	result, err := s.handleChatHistory(c.Request.Context(), nil, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) ginAPIChatSend(c *gin.Context) {
	var body struct {
		Text       string `json:"text"`
		SessionKey string `json:"sessionKey"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text required"})
		return
	}
	if body.SessionKey == "" {
		body.SessionKey = "webchat:default:manager"
	}
	params, _ := json.Marshal(struct {
		Text       string `json:"text"`
		SessionKey string `json:"sessionKey"`
	}{body.Text, body.SessionKey})
	result, err := s.handleChatSendHTTP(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) handleChatSendHTTP(ctx context.Context, params json.RawMessage) (any, error) {
	var p ChatSendParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	chatID := p.SessionKey
	if chatID == "" {
		chatID = "webchat:default:manager"
	}
	var eventSink agent.EventSink = func(agent.Event) {}
	return s.runChatSend(ctx, &p, chatID, eventSink)
}
