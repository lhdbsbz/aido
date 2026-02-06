package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StdioTransport communicates with an MCP server via stdin/stdout of a subprocess.
type StdioTransport struct {
	Command string
	Args    []string
	Env     []string // KEY=VALUE pairs
	Dir     string

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	mu      sync.Mutex

	pending   map[int64]chan jsonRPCResponse
	pendingMu sync.Mutex
}

func NewStdioTransport(command string, args []string, env []string, dir string) *StdioTransport {
	return &StdioTransport{
		Command: command,
		Args:    args,
		Env:     env,
		Dir:     dir,
		pending: make(map[int64]chan jsonRPCResponse),
	}
}

func (t *StdioTransport) Start(ctx context.Context) error {
	t.cmd = exec.CommandContext(ctx, t.Command, t.Args...)
	if t.Dir != "" {
		t.cmd.Dir = t.Dir
	}
	if len(t.Env) > 0 {
		t.cmd.Env = append(t.cmd.Environ(), t.Env...)
	}

	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("start MCP process: %w", err)
	}

	t.scanner = bufio.NewScanner(stdout)
	t.scanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024)

	go t.readLoop()

	return nil
}

func (t *StdioTransport) readLoop() {
	for t.scanner.Scan() {
		line := t.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		t.pendingMu.Lock()
		ch, ok := t.pending[resp.ID]
		if ok {
			delete(t.pending, resp.ID)
		}
		t.pendingMu.Unlock()

		if ok {
			ch <- resp
		}
	}
}

func (t *StdioTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := nextRequestID()

	ch := make(chan jsonRPCResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = ch
	t.pendingMu.Unlock()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	t.mu.Lock()
	_, err = t.stdin.Write(data)
	t.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write to MCP: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

func (t *StdioTransport) Notify(ctx context.Context, method string, params any) error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	_, err = t.stdin.Write(data)
	return err
}

func (t *StdioTransport) Close() {
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}
}
