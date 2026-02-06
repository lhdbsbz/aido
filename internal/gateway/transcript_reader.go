package gateway

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"github.com/lhdbsbz/aido/internal/llm"
)

// transcriptReader provides standalone transcript reading for the gateway.
// This avoids a circular dependency with the session package.
type transcriptReader struct {
	path string
}

type transcriptEntry struct {
	Type    string       `json:"type"`
	Message *llm.Message `json:"message,omitempty"`
	Summary string       `json:"summary,omitempty"`
	Timestamp time.Time  `json:"timestamp"`
}

func (t *transcriptReader) load() ([]llm.Message, error) {
	f, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
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
		var entry transcriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		switch entry.Type {
		case "message":
			if entry.Message != nil {
				messages = append(messages, *entry.Message)
			}
		case "compaction":
			messages = []llm.Message{
				llm.SystemMessage("[Previous conversation summary]\n" + entry.Summary),
			}
		}
	}
	return messages, scanner.Err()
}
