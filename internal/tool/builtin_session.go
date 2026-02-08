package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// SessionStatusTool returns current session/agent/model/workspace (read-only). Uses RunInfo from context.
type SessionStatusTool struct{}

func (t *SessionStatusTool) Name() string        { return "session_status" }
func (t *SessionStatusTool) Description() string { return "Return current session key, agent id, model, and workspace (read-only). Use when the model needs to know which session or workspace it is running in." }
func (t *SessionStatusTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

func (t *SessionStatusTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	info, ok := RunInfoFromContext(ctx)
	if !ok {
		return `{"sessionKey":"","agentId":"","model":"","workspace":""}`, nil
	}
	return fmt.Sprintf("sessionKey: %s\nagentId: %s\nmodel: %s\nworkspace: %s",
		info.SessionKey, info.AgentID, info.Model, info.Workspace), nil
}

// RegisterSessionTools registers session-related builtin tools.
func RegisterSessionTools(r *Registry) {
	r.Register(&SessionStatusTool{})
}
