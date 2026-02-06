package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// WebSearchTool performs a web search via a configurable API.
// For now, uses a simple approach that can be replaced with Brave/SerpAPI/etc.
type WebSearchTool struct {
	APIKey string
}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string { return "Search the web for information" }
func (t *WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query"},
			"count": {"type": "integer", "description": "Number of results (default: 5)"}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Query string
		Count int
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.Count <= 0 {
		p.Count = 5
	}

	if t.APIKey == "" {
		return "", fmt.Errorf("web search API key not configured. Set tools.web.searchApiKey in config")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Brave Search API
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
			strings.ReplaceAll(p.Query, " ", "+"), p.Count), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Subscription-Token", t.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("search API error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil // return raw response if parsing fails
	}

	var sb strings.Builder
	for i, r := range result.Web.Results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	if sb.Len() == 0 {
		return "No results found.", nil
	}
	return sb.String(), nil
}

// RegisterWebTools registers web tools.
func RegisterWebTools(r *Registry, searchAPIKey string) {
	r.Register(&WebFetchTool{})
	r.Register(&WebSearchTool{APIKey: searchAPIKey})
}
