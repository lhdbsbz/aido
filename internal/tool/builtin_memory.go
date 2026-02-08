package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultMemoryPath = "MEMORY.md"

// workDirFromContext returns the workspace for the current run (from RunInfo) when present, otherwise fallback (e.g. registered default). So memory tools use the same workspace as the current agent/session.
func workDirFromContext(ctx context.Context, fallback string) string {
	if info, ok := RunInfoFromContext(ctx); ok && info.Workspace != "" {
		return info.Workspace
	}
	return fallback
}

// MemoryGetTool reads a snippet from MEMORY.md or memory/*.md with optional line range. Uses current run workspace from context when available, else the registered default WorkDir.
type MemoryGetTool struct{ WorkDir string }

func (t *MemoryGetTool) Name() string        { return "memory_get" }
func (t *MemoryGetTool) Description() string { return "Read a snippet from MEMORY.md or a file under memory/ with optional from/lines. Use after memory_search to pull only the needed lines. Path is relative to workspace." }
func (t *MemoryGetTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Relative path (e.g. MEMORY.md or memory/notes.md). Default: MEMORY.md"},
			"from": {"type": "integer", "description": "First line number (1-based). Omit to start at line 1"},
			"lines": {"type": "integer", "description": "Number of lines to return. Omit to return from from to end of file"}
		},
		"required": []
	}`)
}

func (t *MemoryGetTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	workDir := workDirFromContext(ctx, t.WorkDir)
	var p struct {
		Path  string `json:"path"`
		From  int    `json:"from"`
		Lines int    `json:"lines"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Path == "" {
		p.Path = defaultMemoryPath
	}
	path, err := resolveUnder(workDir, p.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	lineSlice := strings.Split(content, "\n")
	if p.From < 1 {
		p.From = 1
	}
	start := p.From - 1
	if start >= len(lineSlice) {
		return "", nil
	}
	end := len(lineSlice)
	if p.Lines > 0 {
		end = start + p.Lines
		if end > len(lineSlice) {
			end = len(lineSlice)
		}
	}
	selected := lineSlice[start:end]
	return strings.Join(selected, "\n"), nil
}

// resolveUnder returns an absolute path under workDir. Disallows escaping with ..
func resolveUnder(workDir, rel string) (string, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) {
		relPath, err := filepath.Rel(workDir, clean)
		if err != nil {
			return "", fmt.Errorf("path not under workspace")
		}
		if strings.HasPrefix(relPath, "..") {
			return "", fmt.Errorf("path not under workspace")
		}
		return clean, nil
	}
	abs := filepath.Join(workDir, clean)
	relPath, err := filepath.Rel(workDir, abs)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path not under workspace")
	}
	return abs, nil
}

// MemorySearchTool does simple keyword/regex search over MEMORY.md and memory/*.md. Uses current run workspace from context when available, else the registered default WorkDir.
type MemorySearchTool struct{ WorkDir string }

func (t *MemorySearchTool) Name() string        { return "memory_search" }
func (t *MemorySearchTool) Description() string { return "Search MEMORY.md and memory/*.md for a query (plain text or regex). Returns matching lines with path and line number. Use before memory_get to pull only needed lines." }
func (t *MemorySearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query (plain text or regex pattern)"},
			"path": {"type": "string", "description": "Optional: single file to search (e.g. MEMORY.md). Omit to search MEMORY.md and all memory/*.md"},
			"max_results": {"type": "integer", "description": "Max number of matches to return (default 20)"}
		},
		"required": ["query"]
	}`)
}

func (t *MemorySearchTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	workDir := workDirFromContext(ctx, t.WorkDir)
	var p struct {
		Query     string `json:"query"`
		Path      string `json:"path"`
		MaxResults int   `json:"max_results"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if p.MaxResults <= 0 {
		p.MaxResults = 20
	}
	pattern, err := regexp.Compile(p.Query)
	if err != nil {
		pattern = regexp.MustCompile(regexp.QuoteMeta(p.Query))
	}
	var paths []string
	if p.Path != "" {
		abs, err := resolveUnder(workDir, p.Path)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err == nil {
			paths = []string{abs}
		}
	} else {
		memoryPath := filepath.Join(workDir, defaultMemoryPath)
		if _, err := os.Stat(memoryPath); err == nil {
			paths = append(paths, memoryPath)
		}
		memoryDir := filepath.Join(workDir, "memory")
		if entries, err := os.ReadDir(memoryDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
					paths = append(paths, filepath.Join(memoryDir, e.Name()))
				}
			}
		}
	}
	var results []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(workDir, path)
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if len(results) >= p.MaxResults {
				break
			}
			if pattern.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
			}
		}
		if len(results) >= p.MaxResults {
			break
		}
	}
	if len(results) == 0 {
		return "No matches found.", nil
	}
	return strings.Join(results, "\n"), nil
}

// RegisterMemoryTools registers memory_get and memory_search. workDir is the workspace root.
func RegisterMemoryTools(r *Registry, workDir string) {
	r.Register(&MemoryGetTool{WorkDir: workDir})
	r.Register(&MemorySearchTool{WorkDir: workDir})
}
