package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/lhdbsbz/aido/internal/llm"
)

// Tool is the interface every tool must implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage // JSON Schema
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// Registry manages all available tools (builtin + MCP).
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Execute runs a tool by name with the given parameters.
func (r *Registry) Execute(ctx context.Context, name string, paramsJSON string) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	result, err := t.Execute(ctx, json.RawMessage(paramsJSON))
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error()), nil
	}
	return result, nil
}

// ListToolDefs returns LLM-compatible tool definitions, filtered by policy.
func (r *Registry) ListToolDefs(policy *Policy) []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]llm.ToolDef, 0, len(r.tools))
	for name, t := range r.tools {
		if policy != nil && !policy.IsAllowed(name) {
			continue
		}
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// ListNames returns all registered tool names.
func (r *Registry) ListNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
