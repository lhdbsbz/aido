package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/lhdbsbz/aido/internal/config"
	"github.com/lhdbsbz/aido/internal/session"
	"github.com/lhdbsbz/aido/internal/skills"
)

// Router manages multiple agents and routes messages to the correct one.
type Router struct {
	mu      sync.RWMutex
	loop    *Loop
	store   *session.Store
	locks   map[string]*sync.Mutex // sessionKey → mutex (prevent concurrent runs)
	locksMu sync.Mutex
	skills  map[string][]skills.SkillEntry // agentID → skills
}

func NewRouter(loop *Loop, store *session.Store) *Router {
	return &Router{
		loop:   loop,
		store:  store,
		locks:  make(map[string]*sync.Mutex),
		skills: make(map[string][]skills.SkillEntry),
	}
}

// SetSkills sets loaded skills for an agent.
func (r *Router) SetSkills(agentID string, list []skills.SkillEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[agentID] = list
}

// InboundMessage represents a message from a bridge or client.
type InboundMessage struct {
	AgentID   string // target agent (default: "default")
	Channel   string // source channel (e.g., "telegram")
	ChatID    string // conversation ID
	SenderID  string // sender identifier
	Text      string
	Images    []ImageAttachment
	MessageID string // for dedup
}

type ImageAttachment struct {
	URL    string
	Base64 string
	MIME   string
}

// HandleMessage routes and processes an inbound message. Returns final text, tool steps (if any), and error.
func (r *Router) HandleMessage(ctx context.Context, msg InboundMessage, eventSink EventSink) (string, []ToolStep, error) {
	cfg := config.Get()
	if cfg == nil {
		return "", nil, fmt.Errorf("config not loaded")
	}
	agentID := msg.AgentID
	if cfg.Gateway.CurrentAgent != "" {
		agentID = cfg.Gateway.CurrentAgent
	}
	if agentID == "" {
		agentID = "default"
	}

	r.mu.RLock()
	agentCfg, ok := cfg.Agents[agentID]
	r.mu.RUnlock()

	if !ok {
		return "", nil, fmt.Errorf("agent %q not found", agentID)
	}

	skillDirs := agentCfg.Skills.Dirs
	if len(skillDirs) == 0 {
		ws := agentCfg.Workspace
		if ws == "" {
			ws = filepath.Join(config.ResolveHome(), "workspace")
		}
		skillDirs = []string{filepath.Join(ws, "skills")}
	}
	loadedSkills := skills.LoadFromDirs(skillDirs)

	// Derive session key
	sessionKey := DeriveSessionKey(agentID, msg.Channel, msg.ChatID)

	// Acquire session lock (prevent concurrent runs on same session)
	lock := r.getSessionLock(sessionKey)
	lock.Lock()
	defer lock.Unlock()

	// Get or create session
	r.store.GetOrCreate(sessionKey, agentID)

	// Build session manager
	compactor := session.DefaultCompactor()
	if agentCfg.Compaction.KeepRecentTokens > 0 {
		compactor.KeepRecentTokens = agentCfg.Compaction.KeepRecentTokens
	}
	if agentCfg.Compaction.ReserveTokens > 0 {
		compactor.ReserveTokens = agentCfg.Compaction.ReserveTokens
	}
	if agentCfg.Compaction.ChunkRatio > 0 {
		compactor.ChunkRatio = agentCfg.Compaction.ChunkRatio
	}

	sessionDir := r.store.TranscriptPath(sessionKey)
	_ = sessionDir // transcript path is derived from store
	mgr := session.NewManager(r.store, compactor, sessionKey, agentID)

	// Build system prompt
	workspace := agentCfg.Workspace
	toolDefs := r.loop.Tools.ListToolDefs(r.loop.Policy)

	promptBuilder := &PromptBuilder{
		AgentConfig: &agentCfg,
		AgentID:     agentID,
		ToolDefs:    toolDefs,
		Skills:      loadedSkills,
		Workspace:   workspace,
	}
	systemPrompt := promptBuilder.Build()

	// Convert image attachments
	var images []struct {
		URL    string
		Base64 string
		MIME   string
	}
	for _, img := range msg.Images {
		images = append(images, struct {
			URL    string
			Base64 string
			MIME   string
		}{URL: img.URL, Base64: img.Base64, MIME: img.MIME})
	}

	var llmImages []struct {
		URL    string `json:"url,omitempty"`
		Base64 string `json:"base64,omitempty"`
		MIME   string `json:"mime,omitempty"`
	}
	_ = llmImages

	// Run agent
	slog.Info("agent run started", "agent", agentID, "session", sessionKey, "channel", msg.Channel)
	start := time.Now()

	var toolSteps []ToolStep
	result, err := r.loop.Run(ctx, RunParams{
		SessionMgr:   mgr,
		AgentConfig:  &agentCfg,
		SystemPrompt: systemPrompt,
		UserMessage:  msg.Text,
		EventSink:    eventSink,
		ToolSteps:    &toolSteps,
	})

	duration := time.Since(start)
	if err != nil {
		slog.Error("agent run failed", "agent", agentID, "session", sessionKey, "error", err, "duration", duration)
		return "", nil, err
	}

	slog.Info("agent run completed", "agent", agentID, "session", sessionKey, "duration", duration)

	// Save session metadata
	if err := r.store.Save(); err != nil {
		slog.Warn("failed to save session store", "error", err)
	}

	return result, toolSteps, nil
}

// DeriveSessionKey creates a deterministic session key.
func DeriveSessionKey(agentID, channel, chatID string) string {
	if channel == "" {
		channel = "direct"
	}
	if chatID == "" {
		chatID = "main"
	}
	return fmt.Sprintf("%s:%s:%s", agentID, channel, chatID)
}

// Store returns the session store for external access.
func (r *Router) Store() *session.Store {
	return r.store
}

func (r *Router) getSessionLock(key string) *sync.Mutex {
	r.locksMu.Lock()
	defer r.locksMu.Unlock()
	if m, ok := r.locks[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	r.locks[key] = m
	return m
}
