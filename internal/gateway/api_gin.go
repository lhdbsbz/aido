package gateway

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lhdbsbz/aido/internal/agent"
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
	sessionKey := c.Query("sessionKey")
	if sessionKey == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "sessionKey required"})
		return
	}
	result, err := s.getChatHistory(c.Request.Context(), sessionKey)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) ginAPIChatSend(c *gin.Context) {
	var body ChatSendParams
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.Text == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "text required"})
		return
	}
	chatID := body.SessionKey
	if chatID == "" {
		chatID = "webchat:default:manager"
	}
	var eventSink agent.EventSink = func(agent.Event) {}
	result, err := s.runChatSend(c.Request.Context(), &body, chatID, eventSink)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
