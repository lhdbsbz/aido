package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/config"
)

//go:embed web/index.html web/static/*
var webFS embed.FS

var upgrader = websocket.Upgrader{
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Server is the Aido gateway server.
type Server struct {
	Config     *config.Config
	ConfigPath string
	Router     *agent.Router
	Conns      *ConnManager
	httpSrv    *http.Server
	startAt    time.Time
}

func NewServer(cfg *config.Config, router *agent.Router, configPath string) *Server {
	s := &Server{
		Config:     cfg,
		ConfigPath: configPath,
		Router:     router,
		Conns:      NewConnManager(),
		startAt:    time.Now(),
	}
	return s
}

// Start begins listening for connections.
func (s *Server) Start(ctx context.Context) error {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	engine.GET("/health", s.ginHealth)
	engine.GET("/ws", s.ginWebSocket)
	s.registerAPIRoutes(engine)
	s.RegisterOpenAICompat(engine)

	webRoot, _ := fs.Sub(webFS, "web")
	staticFS, _ := fs.Sub(webFS, "web/static")
	engine.StaticFS("/static", http.FS(staticFS))
	engine.GET("/", s.ginWebIndex(webRoot))

	addr := fmt.Sprintf(":%d", s.Config.Gateway.Port)
	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: engine,
	}

	slog.Info("Aido gateway starting", "port", s.Config.Gateway.Port)
	uiURL := fmt.Sprintf("http://localhost:%d/", s.Config.Gateway.Port)
	if t := s.Config.Gateway.Auth.Token; t != "" {
		uiURL += "#token=" + url.QueryEscape(t)
	}
	slog.Info("management UI", "url", uiURL)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(shutdownCtx)
	}()

	if err := s.httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) ginWebIndex(webRoot fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path != "/" {
			c.Status(http.StatusNotFound)
			return
		}
		data, err := fs.ReadFile(webRoot, "index.html")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}
}

func (s *Server) ginHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"uptime":  time.Since(s.startAt).String(),
		"bridges": len(s.Conns.ListBridges()),
		"clients": s.Conns.ClientCount(),
	})
}

func (s *Server) ginWebSocket(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	connID := fmt.Sprintf("conn_%d", time.Now().UnixNano())
	conn := &Conn{
		ID:          connID,
		WS:          ws,
		ConnectedAt: time.Now(),
	}

	// First message must be a connect request
	frame, err := ReadFrame(ws)
	if err != nil {
		slog.Warn("failed to read connect frame", "error", err)
		return
	}
	if frame.Method != "connect" {
		conn.Send(ResErr(frame.ID, "HANDSHAKE_REQUIRED", "first message must be a connect request"))
		return
	}

	var connectParams ConnectParams
	if err := json.Unmarshal(frame.Params, &connectParams); err != nil {
		conn.Send(ResErr(frame.ID, "INVALID_PARAMS", "invalid connect params"))
		return
	}

	// Authenticate
	if !s.authenticate(connectParams.Token) {
		conn.Send(ResErr(frame.ID, "AUTH_FAILED", "invalid token"))
		return
	}

	conn.Role = connectParams.Role
	conn.Channel = connectParams.Channel
	conn.Capabilities = connectParams.Capabilities
	s.Conns.Add(conn)
	defer s.Conns.Remove(connID)

	slog.Info("connection established", "id", connID, "role", conn.Role, "channel", conn.Channel)

	// Send hello-ok
	conn.Send(ResOK(frame.ID, map[string]any{
		"connId":   connID,
		"protocol": 1,
	}))

	// Message loop
	for {
		frame, err := ReadFrame(ws)
		if err != nil {
			slog.Debug("connection closed", "id", connID, "error", err)
			return
		}

		if frame.Type != "req" {
			continue
		}

		if frame.Method != "inbound.message" {
			conn.Send(ResErr(frame.ID, "UNKNOWN_METHOD", "use HTTP /api for management; only inbound.message is supported over WebSocket"))
			continue
		}

		go func(f Frame) {
			ctx := context.Background()
			result, err := s.handleInboundMessage(ctx, conn, f.Params)
			if err != nil {
				conn.Send(ResErr(f.ID, "ERROR", err.Error()))
				return
			}
			conn.Send(ResOK(f.ID, result))
		}(frame)
	}
}

func (s *Server) authenticate(token string) bool {
	expected := s.Config.Gateway.Auth.Token
	if expected == "" {
		return true // no auth configured
	}
	return token == expected
}
