package gateway

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Conn represents a single WebSocket connection.
type Conn struct {
	ID           string
	Role         string   // "bridge" | "client"
	Channel      string   // bridge only: channel name
	Capabilities []string // bridge only
	WS           *websocket.Conn
	writeMu      sync.Mutex
	ConnectedAt  time.Time
}

// Send writes a frame to the WebSocket connection (thread-safe).
func (c *Conn) Send(frame Frame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.WS.WriteJSON(frame)
}

// ConnManager tracks all active WebSocket connections.
type ConnManager struct {
	mu    sync.RWMutex
	conns map[string]*Conn // connID → conn
	seq   int
}

func NewConnManager() *ConnManager {
	return &ConnManager{conns: make(map[string]*Conn)}
}

// Add registers a new connection.
func (m *ConnManager) Add(conn *Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[conn.ID] = conn
}

// Remove unregisters a connection.
func (m *ConnManager) Remove(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, connID)
}

// Get returns a connection by ID.
func (m *ConnManager) Get(connID string) *Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conns[connID]
}

// Broadcast sends an event to all connections.
func (m *ConnManager) Broadcast(event string, payload any) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.seq++
	frame := EventFrame(event, m.seq, payload)

	for _, conn := range m.conns {
		if err := conn.Send(frame); err != nil {
			slog.Warn("broadcast failed", "conn", conn.ID, "error", err)
		}
	}
}

// BroadcastToRole sends an event only to connections with a specific role.
func (m *ConnManager) BroadcastToRole(role, event string, payload any) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.seq++
	frame := EventFrame(event, m.seq, payload)

	for _, conn := range m.conns {
		if conn.Role == role {
			if err := conn.Send(frame); err != nil {
				slog.Warn("broadcast failed", "conn", conn.ID, "error", err)
			}
		}
	}
}

// BroadcastToChannel sends an event to bridges of a specific channel.
func (m *ConnManager) BroadcastToChannel(channel, event string, payload any) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.seq++
	frame := EventFrame(event, m.seq, payload)

	for _, conn := range m.conns {
		if conn.Role == RoleBridge && conn.Channel == channel {
			if err := conn.Send(frame); err != nil {
				slog.Warn("broadcast to channel failed", "channel", channel, "conn", conn.ID, "error", err)
			}
		}
	}
}

// ListBridges returns all connected bridge info.
func (m *ConnManager) ListBridges() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var bridges []map[string]any
	for _, conn := range m.conns {
		if conn.Role == RoleBridge {
			bridges = append(bridges, map[string]any{
				"id":           conn.ID,
				"channel":      conn.Channel,
				"capabilities": conn.Capabilities,
				"connectedAt":  conn.ConnectedAt,
			})
		}
	}
	return bridges
}

// ClientCount returns the number of connected clients.
func (m *ConnManager) ClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, conn := range m.conns {
		if conn.Role == RoleClient {
			count++
		}
	}
	return count
}

// ReadFrame reads and parses a WebSocket message into a Frame.
func ReadFrame(ws *websocket.Conn) (Frame, error) {
	var frame Frame
	_, msg, err := ws.ReadMessage()
	if err != nil {
		return frame, err
	}
	err = json.Unmarshal(msg, &frame)
	return frame, err
}
