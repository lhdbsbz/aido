package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lhdbsbz/aido/internal/config"
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
	api.PUT("/config", s.ginAPIConfigPut)
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
	cfg, err := config.Load(config.Path())
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "read config: " + err.Error()})
		return
	}
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.JSON(http.StatusOK, configForUIFromCfg(cfg))
}

func (s *Server) ginAPIConfigPut(c *gin.Context) {
	var cfg config.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid config: " + err.Error()})
		return
	}
	if cfg.Gateway.Port <= 0 || cfg.Gateway.Port > 65535 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "gateway.port must be 1-65535"})
		return
	}
	if err := config.Write(config.Path(), &cfg); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "write config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已保存"})
}

func (s *Server) ginAPISessions(c *gin.Context) {
	result, err := s.handleSessionsList(c.Request.Context(), nil, nil)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) ginAPIChatHistory(c *gin.Context) {
	channel := c.Query("channel")
	channelChatId := c.Query("channelChatId")
	if channel == "" || channelChatId == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "channel and channelChatId required"})
		return
	}
	result, err := s.getChatHistory(c.Request.Context(), channel, channelChatId)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) ginAPIChatSend(c *gin.Context) {
	var body struct {
		Channel        string            `json:"channel"`
		ChannelChatID  string            `json:"channelChatId"`
		Text           string            `json:"text"`
		Attachments    []AttachmentParam `json:"attachments,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.Channel == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "channel required"})
		return
	}
	if body.Text == "" && len(body.Attachments) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "text or at least one attachment required"})
		return
	}
	if body.ChannelChatID == "" {
		body.ChannelChatID = "main"
	}
	params := MessageSendParams{
		Channel:        body.Channel,
		ChannelChatID:  body.ChannelChatID,
		Text:           body.Text,
		Attachments:    body.Attachments,
	}
	result, err := s.handleMessageSend(c.Request.Context(), nil, mustMarshal(params))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
