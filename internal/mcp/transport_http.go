package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

// HTTPTransport communicates with an MCP server over HTTP with Server-Sent Events (SSE).
// Per MCP 2024-11-05: client GETs the SSE endpoint, server sends an "endpoint" event with
// the URI for sending messages; client POSTs JSON-RPC to that URI; server sends responses
// as SSE "message" events.
type HTTPTransport struct {
	SSEURL    string            // SSE endpoint URL (e.g. http://localhost:8080/sse)
	Headers   map[string]string // optional headers (e.g. Authorization)
	postURL   string            // set from endpoint event
	postURLMu sync.RWMutex
	client    *http.Client
	resp      *http.Response
	cancel    context.CancelFunc
	closed    atomic.Bool
	pending   map[int64]chan jsonRPCResponse
	pendingMu sync.Mutex
}

// NewHTTPTransport creates an HTTP+SSE transport. sseURL is the SSE endpoint (GET).
func NewHTTPTransport(sseURL string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{
		SSEURL:  sseURL,
		Headers: headers,
		client:  &http.Client{},
		pending: make(map[int64]chan jsonRPCResponse),
	}
}

// Start connects to the SSE endpoint and waits for the server's "endpoint" event.
func (t *HTTPTransport) Start(ctx context.Context) error {
	if t.closed.Load() {
		return fmt.Errorf("transport already closed")
	}
	ctx, t.cancel = context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.SSEURL, nil)
	if err != nil {
		return fmt.Errorf("build SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range t.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to MCP SSE: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("MCP SSE endpoint returned %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		resp.Body.Close()
		return fmt.Errorf("MCP SSE endpoint Content-Type is %q, expected text/event-stream", ct)
	}
	t.resp = resp

	baseURL, _ := url.Parse(t.SSEURL)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024)
	var eventType string
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if eventType == "endpoint" && len(dataLines) > 0 {
				postURL := strings.TrimSpace(strings.Join(dataLines, "\n"))
				if u, err := url.Parse(postURL); err == nil && u.Scheme == "" && u.Host == "" {
					postURL = baseURL.ResolveReference(u).String()
				}
				t.postURLMu.Lock()
				t.postURL = postURL
				t.postURLMu.Unlock()
				go t.readLoop(scanner)
				return nil
			}
			eventType = ""
			dataLines = nil
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(line[6:])
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line[5:], " "))
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading SSE endpoint event: %w", err)
	}
	return fmt.Errorf("MCP SSE stream ended before endpoint event")
}

func (t *HTTPTransport) readLoop(scanner *bufio.Scanner) {
	var eventType string
	var dataLines []string
	for scanner.Scan() {
		if t.closed.Load() {
			return
		}
		line := scanner.Text()
		if line == "" {
			if eventType == "message" && len(dataLines) > 0 {
				data := []byte(strings.Join(dataLines, "\n"))
				var resp jsonRPCResponse
				if err := json.Unmarshal(data, &resp); err == nil && resp.ID != 0 {
					t.pendingMu.Lock()
					ch, ok := t.pending[resp.ID]
					if ok {
						delete(t.pending, resp.ID)
					}
					t.pendingMu.Unlock()
					if ok {
						select {
						case ch <- resp:
						default:
						}
					}
				}
			}
			eventType = ""
			dataLines = nil
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(line[6:])
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line[5:], " "))
			continue
		}
	}
}

func (t *HTTPTransport) getPostURL() (string, error) {
	t.postURLMu.RLock()
	u := t.postURL
	t.postURLMu.RUnlock()
	if u == "" {
		return "", fmt.Errorf("message endpoint not yet received from server")
	}
	return u, nil
}

// Call sends a JSON-RPC request and waits for the response (delivered via SSE message event per spec).
func (t *HTTPTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	postURL, err := t.getPostURL()
	if err != nil {
		return nil, err
	}
	id := nextRequestID()
	ch := make(chan jsonRPCResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = ch
	t.pendingMu.Unlock()
	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
	}()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("MCP POST: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP POST returned %d", resp.StatusCode)
	}

	select {
	case jsonResp := <-ch:
		if jsonResp.Error != nil {
			return nil, jsonResp.Error
		}
		return jsonResp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (t *HTTPTransport) Notify(ctx context.Context, method string, params any) error {
	postURL, err := t.getPostURL()
	if err != nil {
		return err
	}
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.Headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("MCP POST notify: %w", err)
	}
	resp.Body.Close()
	return nil
}

// Close closes the SSE connection and cancels the context.
func (t *HTTPTransport) Close() {
	t.closed.Store(true)
	if t.cancel != nil {
		t.cancel()
	}
	if t.resp != nil && t.resp.Body != nil {
		t.resp.Body.Close()
	}
}
