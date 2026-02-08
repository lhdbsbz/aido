package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxFetchSize = 200_000

// WebFetchTool fetches content from a URL with support for GET, POST, PUT, DELETE methods.
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string        { return "web_fetch" }
func (t *WebFetchTool) Description() string { return "Fetch content from a URL or send data to an endpoint. Supports GET, POST, PUT, DELETE methods with custom headers and body." }
func (t *WebFetchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "URL to fetch or send to"},
			"method": {"type": "string", "description": "HTTP method: GET (default), POST, PUT, DELETE, PATCH", "enum": ["GET", "POST", "PUT", "DELETE", "PATCH"]},
			"headers": {"type": "object", "description": "Custom headers as key-value pairs"},
			"body": {"type": "string", "description": "Request body (for POST, PUT, PATCH)"},
			"content_type": {"type": "string", "description": "Content-Type header (default: application/json for POST with body)"},
			"timeout": {"type": "integer", "description": "Timeout in seconds (default: 30)"}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		URL         string            `json:"url"`
		Method      string            `json:"method"`
		Headers     map[string]string `json:"headers"`
		Body        string            `json:"body"`
		ContentType string            `json:"content_type"`
		Timeout     int               `json:"timeout"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}

	// Set defaults
	method := strings.ToUpper(p.Method)
	if method == "" {
		method = "GET"
	}
	
	timeout := 30
	if p.Timeout > 0 {
		timeout = p.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var body io.Reader
	if p.Body != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		contentType := p.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		// If body is JSON object, validate it
		if contentType == "application/json" {
			if !json.Valid([]byte(p.Body)) {
				return "", fmt.Errorf("invalid JSON body for Content-Type: application/json")
			}
		}
		body = bytes.NewReader([]byte(p.Body))
	}

	req, err := http.NewRequestWithContext(ctx, method, p.URL, body)
	if err != nil {
		return "", err
	}

	// Set default User-Agent
	req.Header.Set("User-Agent", "Aido/1.0")

	// Set custom headers
	for key, value := range p.Headers {
		req.Header.Set(key, value)
	}

	// Auto-set Content-Type if body exists and not set
	if p.Body != "" && req.Header.Get("Content-Type") == "" {
		contentType := p.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return "", err
	}

	content := string(respBody)
	if len(respBody) >= maxFetchSize {
		content += "\n[...truncated, response too large]"
	}

	// Build response with status and headers
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%d %s]\n", resp.StatusCode, resp.Status))
	sb.WriteString(fmt.Sprintf("Content-Length: %d\n", len(respBody)))
	
	// Include relevant response headers
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		sb.WriteString(fmt.Sprintf("Content-Type: %s\n", contentType))
	}
	
	sb.WriteString("\n")
	sb.WriteString(content)

	return sb.String(), nil
}

// RegisterWebTools registers web tools.
func RegisterWebTools(r *Registry) {
	r.Register(&WebFetchTool{})
}
