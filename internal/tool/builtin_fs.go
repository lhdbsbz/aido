package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileTool reads file contents.
type ReadFileTool struct{ WorkDir string }

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the contents of a file" }
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path to read"}
		},
		"required": ["path"]
	}`)
}
func (t *ReadFileTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct{ Path string }
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	path := t.resolve(p.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > 100_000 {
		return string(data[:100_000]) + "\n[...truncated, file too large]", nil
	}
	return string(data), nil
}
func (t *ReadFileTool) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(t.WorkDir, p)
}

// WriteFileTool creates or overwrites a file.
type WriteFileTool struct{ WorkDir string }

func (t *WriteFileTool) Name() string        { return "write_file" }
func (t *WriteFileTool) Description() string { return "Create or overwrite a file with content" }
func (t *WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path to write"},
			"content": {"type": "string", "description": "Content to write"}
		},
		"required": ["path", "content"]
	}`)
}
func (t *WriteFileTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Path    string
		Content string
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	path := t.resolve(p.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(p.Content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Written %d bytes to %s", len(p.Content), p.Path), nil
}
func (t *WriteFileTool) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(t.WorkDir, p)
}

// EditFileTool performs string replacement in a file.
type EditFileTool struct{ WorkDir string }

func (t *EditFileTool) Name() string        { return "edit_file" }
func (t *EditFileTool) Description() string { return "Replace exact string occurrences in a file" }
func (t *EditFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File path to edit"},
			"old_string": {"type": "string", "description": "Exact string to find"},
			"new_string": {"type": "string", "description": "Replacement string"}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}
func (t *EditFileTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	path := t.resolve(p.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, p.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", p.Path)
	}
	newContent := strings.Replace(content, p.OldString, p.NewString, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Replaced 1 occurrence in %s (%d total found)", p.Path, count), nil
}
func (t *EditFileTool) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(t.WorkDir, p)
}

// ListDirTool lists directory contents.
type ListDirTool struct{ WorkDir string }

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string { return "List files and directories in a path" }
func (t *ListDirTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory path to list (default: current)"}
		}
	}`)
}
func (t *ListDirTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct{ Path string }
	_ = json.Unmarshal(params, &p)
	dir := p.Path
	if dir == "" {
		dir = "."
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(t.WorkDir, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, e := range entries {
		info, _ := e.Info()
		if e.IsDir() {
			fmt.Fprintf(&sb, "  %s/\n", e.Name())
		} else if info != nil {
			fmt.Fprintf(&sb, "  %s (%d bytes)\n", e.Name(), info.Size())
		} else {
			fmt.Fprintf(&sb, "  %s\n", e.Name())
		}
	}
	return sb.String(), nil
}

// GrepTool searches file contents with a pattern.
type GrepTool struct{ WorkDir string }

func (t *GrepTool) Name() string        { return "grep" }
func (t *GrepTool) Description() string { return "Search for a pattern in files under a directory" }
func (t *GrepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Text pattern to search for"},
			"path": {"type": "string", "description": "Directory or file to search (default: current)"},
			"include": {"type": "string", "description": "File glob to include (e.g. *.go)"}
		},
		"required": ["pattern"]
	}`)
}
func (t *GrepTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Pattern string
		Path    string
		Include string
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	dir := p.Path
	if dir == "" {
		dir = "."
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(t.WorkDir, dir)
	}

	var sb strings.Builder
	matchCount := 0
	maxMatches := 100

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || matchCount >= maxMatches {
			return nil
		}
		if p.Include != "" {
			matched, _ := filepath.Match(p.Include, info.Name())
			if !matched {
				return nil
			}
		}
		if info.Size() > 1_000_000 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		relPath, _ := filepath.Rel(t.WorkDir, path)
		for i, line := range lines {
			if strings.Contains(line, p.Pattern) {
				fmt.Fprintf(&sb, "%s:%d: %s\n", relPath, i+1, strings.TrimSpace(line))
				matchCount++
				if matchCount >= maxMatches {
					sb.WriteString("[...truncated, too many matches]\n")
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if matchCount == 0 {
		return "No matches found.", nil
	}
	return sb.String(), nil
}

// FindFilesTool finds files by glob pattern.
type FindFilesTool struct{ WorkDir string }

func (t *FindFilesTool) Name() string        { return "find_files" }
func (t *FindFilesTool) Description() string { return "Find files matching a glob pattern" }
func (t *FindFilesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Glob pattern (e.g. **/*.go)"},
			"path": {"type": "string", "description": "Base directory (default: current)"}
		},
		"required": ["pattern"]
	}`)
}
func (t *FindFilesTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Pattern string
		Path    string
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	dir := p.Path
	if dir == "" {
		dir = "."
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(t.WorkDir, dir)
	}

	var sb strings.Builder
	count := 0
	maxFiles := 200

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || count >= maxFiles {
			return nil
		}
		matched, _ := filepath.Match(p.Pattern, info.Name())
		if matched {
			relPath, _ := filepath.Rel(t.WorkDir, path)
			fmt.Fprintf(&sb, "%s\n", relPath)
			count++
			if count >= maxFiles {
				sb.WriteString("[...truncated]\n")
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "No files found.", nil
	}
	return sb.String(), nil
}

// RegisterFSTools registers all filesystem tools.
func RegisterFSTools(r *Registry, workDir string) {
	r.Register(&ReadFileTool{WorkDir: workDir})
	r.Register(&WriteFileTool{WorkDir: workDir})
	r.Register(&EditFileTool{WorkDir: workDir})
	r.Register(&ListDirTool{WorkDir: workDir})
	r.Register(&GrepTool{WorkDir: workDir})
	r.Register(&FindFilesTool{WorkDir: workDir})
}
