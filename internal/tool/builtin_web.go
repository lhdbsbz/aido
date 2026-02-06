package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxFetchSize = 200_000

// WebFetchTool fetches content from a URL.
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string        { return "web_fetch" }
func (t *WebFetchTool) Description() string { return "Fetch and return content from a URL" }
func (t *WebFetchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "URL to fetch"}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct{ URL string }
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Aido/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return "", err
	}

	content := string(body)
	if len(body) >= maxFetchSize {
		content += "\n[...truncated, response too large]"
	}

	return fmt.Sprintf("[%d %s]\n%s", resp.StatusCode, resp.Status, content), nil
}

// RegisterWebTools registers web tools.
func RegisterWebTools(r *Registry) {
	r.Register(&WebFetchTool{})
}
