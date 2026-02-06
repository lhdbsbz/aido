package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/config"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Server is the Aido gateway server.
type Server struct {
	Config  *config.Config
	Router  *agent.Router
	Conns   *ConnManager
	methods map[string]MethodHandler
	httpSrv *http.Server
	startAt time.Time
}

// MethodHandler processes a gateway RPC method.
type MethodHandler func(ctx context.Context, conn *Conn, params json.RawMessage) (any, error)

func NewServer(cfg *config.Config, router *agent.Router) *Server {
	s := &Server{
		Config:  cfg,
		Router:  router,
		Conns:   NewConnManager(),
		methods: make(map[string]MethodHandler),
		startAt: time.Now(),
	}
	s.registerMethods()
	return s
}

// Start begins listening for connections.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	// OpenAI compat will be registered in Phase 4 openai.go

	addr := fmt.Sprintf(":%d", s.Config.Gateway.Port)
	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	slog.Info("Aido gateway starting", "port", s.Config.Gateway.Port)

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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"uptime":  time.Since(s.startAt).String(),
		"bridges": len(s.Conns.ListBridges()),
		"clients": s.Conns.ClientCount(),
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
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

		handler, ok := s.methods[frame.Method]
		if !ok {
			conn.Send(ResErr(frame.ID, "UNKNOWN_METHOD", fmt.Sprintf("unknown method: %s", frame.Method)))
			continue
		}

		go func(f Frame) {
			ctx := context.Background()
			result, err := handler(ctx, conn, f.Params)
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
