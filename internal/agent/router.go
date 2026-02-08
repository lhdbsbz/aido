package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lhdbsbz/aido/internal/config"
	"github.com/lhdbsbz/aido/internal/prompts"
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
// User message = text + attachments; original media is delivered to the model when supported.
type InboundMessage struct {
	AgentID     string       // target agent (default: "default")
	Channel     string       // source channel (e.g., "telegram")
	ChatID      string       // conversation ID
	SenderID    string       // sender identifier
	Text        string
	Attachments []Attachment // image | audio | video | file
	MessageID   string       // for dedup
}

// Attachment is one media or file item. Type is "image" | "audio" | "video" | "file".
// Content is either URL or Base64+MIME.
type Attachment struct {
	Type   string
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

	loadedSkills := skills.LoadFromDirs([]string{config.SkillsDir()})

	// Session key = channel:channelChatId (no agentId; switch agent config does not change session)
	sessionKey := SessionKeyFromChannelChat(msg.Channel, msg.ChatID)

	// Acquire session lock (prevent concurrent runs on same session)
	lock := r.getSessionLock(sessionKey)
	lock.Lock()
	defer lock.Unlock()

	// Get or create session
	r.store.GetOrCreate(sessionKey, agentID)

	locale := cfg.Gateway.Locale
	if locale != "en" {
		locale = "zh"
	}
	p := prompts.Get(locale)

	compactor := session.DefaultCompactor()
	compactor.SummarizePromptTemplate = p.SummarizePromptTemplate
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

	toolDefs := r.loop.Tools.ListToolDefs()
	workspace := config.Workspace()

	promptBuilder := &PromptBuilder{
		Prompts:     p,
		AgentConfig: &agentCfg,
		AgentID:     agentID,
		ToolDefs:    toolDefs,
		Skills:      loadedSkills,
		Workspace:   workspace,
	}
	systemPrompt := promptBuilder.Build()

	slog.Info("agent run started", "agent", agentID, "session", sessionKey, "channel", msg.Channel)
	start := time.Now()

	var toolSteps []ToolStep
	result, err := r.loop.Run(ctx, RunParams{
		SessionMgr:   mgr,
		AgentID:      agentID,
		AgentConfig:  &agentCfg,
		SystemPrompt: systemPrompt,
		UserMessage:  msg.Text,
		Attachments:  msg.Attachments,
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

// SessionKeyFromChannelChat creates the session key for storage/lock: channel:channelChatId only.
func SessionKeyFromChannelChat(channel, chatID string) string {
	if channel == "" {
		channel = "direct"
	}
	if chatID == "" {
		chatID = "main"
	}
	return channel + ":" + chatID
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
