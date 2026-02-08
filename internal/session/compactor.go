package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lhdbsbz/aido/internal/llm"
	"github.com/lhdbsbz/aido/internal/prompts"
)

// Compactor handles session context window management via LLM summarization.
type Compactor struct {
	KeepRecentTokens         int     // tokens to keep at the end (default: 20000)
	ReserveTokens            int     // tokens to reserve for new content (default: 16384)
	ChunkRatio               float64 // ratio for chunking messages to summarize (default: 0.4)
	SafetyMargin             float64 // multiplier for token estimation inaccuracy (default: 1.2)
	SummarizePromptTemplate  string  // prompt template with one %s for conversation body; empty uses default (en)
}

func DefaultCompactor() *Compactor {
	return &Compactor{
		KeepRecentTokens: 20000,
		ReserveTokens:    16384,
		ChunkRatio:       0.4,
		SafetyMargin:     1.2,
	}
}

// ShouldCompact checks if the session needs compaction.
func (c *Compactor) ShouldCompact(messages []llm.Message, contextWindow int) bool {
	currentTokens := int(float64(EstimateMessagesTokens(messages)) * c.SafetyMargin)
	return currentTokens > (contextWindow - c.ReserveTokens)
}

// Compact performs LLM-based summarization of older messages.
// Returns the new message list (summary + recent messages).
func (c *Compactor) Compact(ctx context.Context, client llm.Client, messages []llm.Message, params llm.ChatParams) ([]llm.Message, string, error) {
	splitIdx := c.findSplitIndex(messages)
	if splitIdx <= 0 {
		return messages, "", nil // nothing to compress
	}

	toCompress := messages[:splitIdx]
	toKeep := messages[splitIdx:]

	chunks := c.chunkMessages(toCompress)
	var summaries []string

	for _, chunk := range chunks {
		summary, err := c.summarizeChunk(ctx, client, chunk, params)
		if err != nil {
			return nil, "", fmt.Errorf("summarize chunk: %w", err)
		}
		summaries = append(summaries, summary)
	}

	fullSummary := strings.Join(summaries, "\n\n")

	// Build new message list: summary as system context + recent messages
	newMessages := make([]llm.Message, 0, len(toKeep)+1)
	newMessages = append(newMessages, llm.SystemMessage("[Previous conversation summary]\n"+fullSummary))
	newMessages = append(newMessages, toKeep...)

	return newMessages, fullSummary, nil
}

// findSplitIndex finds where to split: keep the most recent KeepRecentTokens.
func (c *Compactor) findSplitIndex(messages []llm.Message) int {
	totalTokens := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i].Content) + 4
		for _, tc := range messages[i].ToolCalls {
			msgTokens += EstimateTokens(tc.Arguments)
		}
		totalTokens += msgTokens
		if totalTokens >= c.KeepRecentTokens {
			return i + 1
		}
	}
	return 0 // everything fits in the keep window
}

// chunkMessages splits messages into chunks for summarization.
func (c *Compactor) chunkMessages(messages []llm.Message) [][]llm.Message {
	if len(messages) == 0 {
		return nil
	}

	totalTokens := EstimateMessagesTokens(messages)
	chunkSize := int(float64(totalTokens) * c.ChunkRatio)
	if chunkSize < 2000 {
		chunkSize = 2000
	}

	var chunks [][]llm.Message
	var currentChunk []llm.Message
	currentTokens := 0

	for _, msg := range messages {
		msgTokens := EstimateTokens(msg.Content) + 4
		if currentTokens+msgTokens > chunkSize && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentTokens = 0
		}
		currentChunk = append(currentChunk, msg)
		currentTokens += msgTokens
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// summarizeChunk calls the LLM to summarize a chunk of messages.
func (c *Compactor) summarizeChunk(ctx context.Context, client llm.Client, messages []llm.Message, baseParams llm.ChatParams) (string, error) {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
		for _, tc := range msg.ToolCalls {
			sb.WriteString(fmt.Sprintf("  [tool_call %s]: %s(%s)\n", tc.ID, tc.Name, truncate(tc.Arguments, 200)))
		}
	}

	tmpl := c.SummarizePromptTemplate
	if tmpl == "" {
		tmpl = prompts.Get("zh").SummarizePromptTemplate
	}
	prompt := fmt.Sprintf(tmpl, sb.String())

	stream, err := client.Chat(ctx, llm.ChatParams{
		Provider: baseParams.Provider,
		Model:    baseParams.Model,
		APIKey:   baseParams.APIKey,
		BaseURL:  baseParams.BaseURL,
		Messages: []llm.Message{llm.UserMessage(prompt)},
	})
	if err != nil {
		return "", err
	}

	result, err := llm.ConsumeStream(ctx, stream)
	if err != nil {
		return "", err
	}

	return result.Text, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Manager ties together Store, Transcript, and Compactor for a single session.
type Manager struct {
	Store      *Store
	Compactor  *Compactor
	sessionKey string
	agentID    string
	transcript *Transcript
}

func NewManager(store *Store, compactor *Compactor, sessionKey, agentID string) *Manager {
	transcriptPath := store.TranscriptPath(sessionKey)
	return &Manager{
		Store:      store,
		Compactor:  compactor,
		sessionKey: sessionKey,
		agentID:    agentID,
		transcript: NewTranscript(transcriptPath),
	}
}

func (m *Manager) SessionKey() string { return m.sessionKey }

func (m *Manager) LoadTranscript() ([]llm.Message, error) {
	return m.transcript.Load()
}

func (m *Manager) Append(msg llm.Message) error {
	return m.transcript.Append(msg)
}

func (m *Manager) ShouldCompact(contextWindow int) (bool, error) {
	messages, err := m.transcript.Load()
	if err != nil {
		return false, err
	}
	return m.Compactor.ShouldCompact(messages, contextWindow), nil
}

func (m *Manager) DoCompact(ctx context.Context, client llm.Client, params llm.ChatParams, contextWindow int) error {
	messages, err := m.transcript.Load()
	if err != nil {
		return err
	}

	if !m.Compactor.ShouldCompact(messages, contextWindow) {
		return nil
	}

	newMessages, summary, err := m.Compactor.Compact(ctx, client, messages, params)
	if err != nil {
		return fmt.Errorf("compact: %w", err)
	}

	if summary == "" {
		return nil
	}

	// Rewrite transcript: compaction entry + remaining messages
	entries := make([]TranscriptEntry, 0, len(newMessages)+1)
	entries = append(entries, TranscriptEntry{
		Type:      "compaction",
		ID:        fmt.Sprintf("c%d", time.Now().UnixMilli()),
		Timestamp: time.Now(),
		Summary:   summary,
	})
	for _, msg := range newMessages[1:] { // skip the summary system message (it's in the compaction entry)
		entries = append(entries, TranscriptEntry{
			Type:      "message",
			ID:        fmt.Sprintf("m%d", time.Now().UnixMilli()),
			Timestamp: time.Now(),
			Message:   &msg,
		})
	}

	if err := m.transcript.Rewrite(entries); err != nil {
		return fmt.Errorf("rewrite transcript: %w", err)
	}

	// Update metadata
	m.Store.GetOrCreate(m.sessionKey, m.agentID)
	entry := m.Store.Get(m.sessionKey)
	if entry != nil {
		entry.Compactions++
		entry.UpdatedAt = time.Now()
	}

	return m.Store.Save()
}
