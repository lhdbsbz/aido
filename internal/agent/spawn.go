package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SubAgent represents a background agent run.
type SubAgent struct {
	ID         string
	SessionKey string
	AgentID    string
	Message    string
	StartedAt  time.Time
	Done       bool
	Result     string
	Error      error
	cancel     context.CancelFunc
}

// SpawnManager manages sub-agent goroutines.
type SpawnManager struct {
	mu       sync.RWMutex
	router   *Router
	running  map[string]*SubAgent
	maxConc  int
}

func NewSpawnManager(router *Router, maxConcurrent int) *SpawnManager {
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	return &SpawnManager{
		router:  router,
		running: make(map[string]*SubAgent),
		maxConc: maxConcurrent,
	}
}

// Spawn starts a sub-agent in a background goroutine.
func (m *SpawnManager) Spawn(parentCtx context.Context, agentID, message string, eventSink EventSink) (string, error) {
	m.mu.RLock()
	activeCount := 0
	for _, sa := range m.running {
		if !sa.Done {
			activeCount++
		}
	}
	m.mu.RUnlock()

	if activeCount >= m.maxConc {
		return "", fmt.Errorf("max concurrent sub-agents reached (%d)", m.maxConc)
	}

	subID := fmt.Sprintf("sub_%d", time.Now().UnixMilli())
	ctx, cancel := context.WithCancel(context.Background())

	sa := &SubAgent{
		ID:        subID,
		AgentID:   agentID,
		Message:   message,
		StartedAt: time.Now(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.running[subID] = sa
	m.mu.Unlock()

	go func() {
		defer cancel()
		result, _, err := m.router.HandleMessage(ctx, InboundMessage{
			AgentID: agentID,
			Channel: "subagent",
			ChatID:  subID,
			Text:    message,
		}, eventSink)

		m.mu.Lock()
		sa.Done = true
		sa.Result = result
		sa.Error = err
		m.mu.Unlock()

		if err != nil {
			slog.Warn("sub-agent failed", "id", subID, "error", err)
		} else {
			slog.Info("sub-agent completed", "id", subID)
		}
	}()

	return subID, nil
}

// List returns all sub-agents (active and completed).
func (m *SpawnManager) List() []*SubAgent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*SubAgent, 0, len(m.running))
	for _, sa := range m.running {
		result = append(result, sa)
	}
	return result
}

// Stop cancels a running sub-agent.
func (m *SpawnManager) Stop(subID string) error {
	m.mu.RLock()
	sa, ok := m.running[subID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("sub-agent %s not found", subID)
	}
	if sa.Done {
		return fmt.Errorf("sub-agent %s already completed", subID)
	}
	sa.cancel()
	return nil
}

// Cleanup removes completed sub-agents older than the given duration.
func (m *SpawnManager) Cleanup(olderThan time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	for id, sa := range m.running {
		if sa.Done && sa.StartedAt.Before(cutoff) {
			delete(m.running, id)
		}
	}
}
