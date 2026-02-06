package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lhdbsbz/aido/internal/llm"
)

// TranscriptEntry is a single line in the JSONL transcript file.
type TranscriptEntry struct {
	Type      string      `json:"type"`                // "message" | "compaction"
	ID        string      `json:"id"`
	Timestamp time.Time   `json:"timestamp"`
	Message   *llm.Message `json:"message,omitempty"`
	Summary   string      `json:"summary,omitempty"`   // for compaction entries
}

// Transcript manages append-only JSONL transcript files.
type Transcript struct {
	path string
}

func NewTranscript(path string) *Transcript {
	return &Transcript{path: path}
}

func (t *Transcript) Path() string { return t.path }

// Append writes a message entry to the transcript file.
func (t *Transcript) Append(msg llm.Message) error {
	entry := TranscriptEntry{
		Type:      "message",
		ID:        fmt.Sprintf("m%d", time.Now().UnixMilli()),
		Timestamp: time.Now(),
		Message:   &msg,
	}
	return t.appendEntry(entry)
}

// AppendCompaction writes a compaction summary entry.
func (t *Transcript) AppendCompaction(summary string) error {
	entry := TranscriptEntry{
		Type:      "compaction",
		ID:        fmt.Sprintf("c%d", time.Now().UnixMilli()),
		Timestamp: time.Now(),
		Summary:   summary,
	}
	return t.appendEntry(entry)
}

func (t *Transcript) appendEntry(entry TranscriptEntry) error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0755); err != nil {
		return fmt.Errorf("create transcript dir: %w", err)
	}

	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	data = append(data, '\n')

	_, err = f.Write(data)
	return err
}

// Load reads all entries from the transcript and returns conversation messages.
// Compaction entries replace all preceding messages with a summary.
func (t *Transcript) Load() ([]llm.Message, error) {
	f, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var messages []llm.Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry TranscriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}

		switch entry.Type {
		case "message":
			if entry.Message != nil {
				messages = append(messages, *entry.Message)
			}
		case "compaction":
			// Compaction replaces all previous messages with a summary
			messages = []llm.Message{
				llm.SystemMessage("[Previous conversation summary]\n" + entry.Summary),
			}
		}
	}

	return messages, scanner.Err()
}

// Rewrite replaces the entire transcript with new content.
// Used after compaction.
func (t *Transcript) Rewrite(entries []TranscriptEntry) error {
	tmpPath := t.path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp transcript: %w", err)
	}
	defer f.Close()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		data = append(data, '\n')
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, t.path)
}
