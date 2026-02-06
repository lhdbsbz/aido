package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const maxExecOutput = 50_000

// ExecTool executes shell commands.
type ExecTool struct {
	WorkDir string
	Timeout time.Duration
}

func (t *ExecTool) Name() string        { return "exec" }
func (t *ExecTool) Description() string { return "Execute a shell command and return its output" }
func (t *ExecTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "Shell command to execute"},
			"timeout": {"type": "integer", "description": "Timeout in seconds (default: 30)"}
		},
		"required": ["command"]
	}`)
}

func (t *ExecTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Command string
		Timeout int
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if p.Timeout > 0 {
		timeout = time.Duration(p.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", p.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", p.Command)
	}
	cmd.Dir = t.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var sb strings.Builder
	if stdout.Len() > 0 {
		out := stdout.String()
		if len(out) > maxExecOutput {
			out = out[:maxExecOutput] + "\n[...truncated]"
		}
		sb.WriteString(out)
	}
	if stderr.Len() > 0 {
		errOut := stderr.String()
		if len(errOut) > maxExecOutput {
			errOut = errOut[:maxExecOutput] + "\n[...truncated]"
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("[stderr]\n")
		sb.WriteString(errOut)
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return sb.String(), fmt.Errorf("command timed out after %s", timeout)
		}
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if sb.Len() > 0 {
			return fmt.Sprintf("%s\n[exit code: %d]", sb.String(), exitCode), nil
		}
		return "", fmt.Errorf("command failed (exit %d): %s", exitCode, err)
	}

	return sb.String(), nil
}

// RegisterExecTools registers execution tools.
func RegisterExecTools(r *Registry, workDir string) {
	r.Register(&ExecTool{WorkDir: workDir, Timeout: 30 * time.Second})
}
