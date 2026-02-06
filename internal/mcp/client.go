package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/lhdbsbz/aido/internal/llm"
	toolpkg "github.com/lhdbsbz/aido/internal/tool"
)

// Client manages connections to MCP servers and exposes their tools.
type Client struct {
	mu         sync.RWMutex
	transports map[string]Transport // server name → transport
	tools      map[string]*MCPTool  // tool name → tool (with server reference)
}

func NewClient() *Client {
	return &Client{
		transports: make(map[string]Transport),
		tools:      make(map[string]*MCPTool),
	}
}

// AddServer connects to an MCP server and discovers its tools.
func (c *Client) AddServer(ctx context.Context, name string, transport Transport) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := transport.Start(ctx); err != nil {
		return fmt.Errorf("start MCP server %s: %w", name, err)
	}

	// Initialize
	_, err := transport.Call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "aido",
			"version": "0.1.0",
		},
	})
	if err != nil {
		transport.Close()
		return fmt.Errorf("initialize MCP server %s: %w", name, err)
	}

	// Send initialized notification
	_ = transport.Notify(ctx, "notifications/initialized", nil)

	// Discover tools
	result, err := transport.Call(ctx, "tools/list", nil)
	if err != nil {
		transport.Close()
		return fmt.Errorf("list tools from %s: %w", name, err)
	}

	var toolList struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &toolList); err != nil {
		transport.Close()
		return fmt.Errorf("parse tools from %s: %w", name, err)
	}

	c.transports[name] = transport
	for _, t := range toolList.Tools {
		fullName := name + ":" + t.Name
		c.tools[fullName] = &MCPTool{
			serverName:  name,
			toolName:    t.Name,
			fullName:    fullName,
			description: t.Description,
			parameters:  t.InputSchema,
			transport:   transport,
		}
	}

	return nil
}

// RegisterTools adds all MCP tools to a tool registry.
func (c *Client) RegisterTools(registry *toolpkg.Registry) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, t := range c.tools {
		registry.Register(t)
	}
}

// Close shuts down all MCP server connections.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.transports {
		t.Close()
	}
}

// ToolNames returns all discovered MCP tool names.
func (c *Client) ToolNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.tools))
	for name := range c.tools {
		names = append(names, name)
	}
	return names
}

// MCPTool wraps an MCP server tool as a tool.Tool.
type MCPTool struct {
	serverName  string
	toolName    string
	fullName    string
	description string
	parameters  json.RawMessage
	transport   Transport
}

func (t *MCPTool) Name() string               { return t.fullName }
func (t *MCPTool) Description() string         { return t.description }
func (t *MCPTool) Parameters() json.RawMessage { return t.parameters }

func (t *MCPTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args any
	if len(params) > 0 {
		_ = json.Unmarshal(params, &args)
	}

	result, err := t.transport.Call(ctx, "tools/call", map[string]any{
		"name":      t.toolName,
		"arguments": args,
	})
	if err != nil {
		return "", fmt.Errorf("MCP tool %s: %w", t.fullName, err)
	}

	var callResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &callResult); err != nil {
		return string(result), nil
	}

	var texts []string
	for _, c := range callResult.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}

	text := joinStrings(texts, "\n")
	if callResult.IsError {
		return "", fmt.Errorf("%s", text)
	}
	return text, nil
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}

// Transport is the interface for MCP server communication.
type Transport interface {
	Start(ctx context.Context) error
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Notify(ctx context.Context, method string, params any) error
	Close()
}

// JSON-RPC types for MCP protocol

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

var requestIDCounter atomic.Int64

func nextRequestID() int64 {
	return requestIDCounter.Add(1)
}

// LLMToolDefs returns MCP tools as LLM-compatible tool definitions.
func (c *Client) LLMToolDefs() []llm.ToolDef {
	c.mu.RLock()
	defer c.mu.RUnlock()
	defs := make([]llm.ToolDef, 0, len(c.tools))
	for _, t := range c.tools {
		defs = append(defs, llm.ToolDef{
			Name:        t.fullName,
			Description: t.description,
			Parameters:  t.parameters,
		})
	}
	return defs
}
