package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry holds metadata for a single session.
type Entry struct {
	SessionKey   string    `json:"sessionKey"`
	AgentID      string    `json:"agentId"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	InputTokens  int       `json:"inputTokens"`
	OutputTokens int       `json:"outputTokens"`
	Compactions  int       `json:"compactions"`
}

// Store manages session metadata and provides session lookup/creation.
type Store struct {
	mu       sync.RWMutex
	baseDir  string
	sessions map[string]*Entry // sessionKey → entry
}

func NewStore(baseDir string) *Store {
	return &Store{
		baseDir:  baseDir,
		sessions: make(map[string]*Entry),
	}
}

// Load reads session metadata from disk.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metaPath := s.metaPath()
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read session store: %w", err)
	}

	var entries map[string]*Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse session store: %w", err)
	}
	s.sessions = entries
	return nil
}

// Save persists session metadata to disk (atomic write).
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metaPath := s.metaPath()
	if err := os.MkdirAll(filepath.Dir(metaPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session store: %w", err)
	}

	tmpPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write session store: %w", err)
	}
	return os.Rename(tmpPath, metaPath)
}

// Get returns an existing session entry, or nil if not found.
func (s *Store) Get(sessionKey string) *Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionKey]
}

// GetOrCreate returns an existing session or creates a new one.
func (s *Store) GetOrCreate(sessionKey, agentID string) *Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.sessions[sessionKey]; ok {
		return entry
	}

	entry := &Entry{
		SessionKey: sessionKey,
		AgentID:    agentID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	s.sessions[sessionKey] = entry
	return entry
}

// List returns all session entries.
func (s *Store) List() []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]*Entry, 0, len(s.sessions))
	for _, e := range s.sessions {
		entries = append(entries, e)
	}
	return entries
}

// Delete removes a session entry and its transcript.
func (s *Store) Delete(sessionKey string) error {
	s.mu.Lock()
	delete(s.sessions, sessionKey)
	s.mu.Unlock()

	transcriptPath := s.TranscriptPath(sessionKey)
	if err := os.Remove(transcriptPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return s.Save()
}

// UpdateUsage adds token usage to a session entry.
func (s *Store) UpdateUsage(sessionKey string, input, output int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.sessions[sessionKey]; ok {
		entry.InputTokens += input
		entry.OutputTokens += output
		entry.UpdatedAt = time.Now()
	}
}

// TranscriptPath returns the file path for a session's transcript.
func (s *Store) TranscriptPath(sessionKey string) string {
	return filepath.Join(s.baseDir, safeFileName(sessionKey)+".jsonl")
}

func (s *Store) metaPath() string {
	return filepath.Join(s.baseDir, "meta.json")
}

// safeFileName converts a session key to a safe filename.
func safeFileName(key string) string {
	safe := make([]byte, 0, len(key))
	for _, c := range []byte(key) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			safe = append(safe, c)
		} else {
			safe = append(safe, '_')
		}
	}
	return string(safe)
}
