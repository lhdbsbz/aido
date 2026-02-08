package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lhdbsbz/aido/internal/config"
	"github.com/lhdbsbz/aido/internal/llm"
	"github.com/lhdbsbz/aido/internal/session"
	"github.com/lhdbsbz/aido/internal/tool"
)

var (
	ErrMaxIterations = errors.New("max tool call iterations reached")
	ErrAborted       = errors.New("agent run aborted")
)

const (
	DefaultMaxIterations = 50
	DefaultContextWindow = 200_000
)

// Loop is the core agent execution engine.
type Loop struct {
	OpenAI    *llm.OpenAIClient
	Anthropic *llm.AnthropicClient
	Tools     *tool.Registry
	Policy    *tool.Policy
	Config    *config.Config

	MaxIterations int
	ContextWindow int
}

// SetPolicy updates the tool policy (e.g. after config hot-reload).
func (l *Loop) SetPolicy(p *tool.Policy) {
	l.Policy = p
}

// RunParams holds parameters for a single agent run.
// Attachments are converted to LLM content in one place: image -> image blocks; others noted in text.
type RunParams struct {
	SessionMgr   *session.Manager
	AgentConfig  *config.AgentConfig
	SystemPrompt string
	UserMessage  string
	Attachments  []Attachment
	EventSink    EventSink
	ToolSteps    *[]ToolStep // optional: collect tool steps for API response
}

// Run executes one complete agent turn: LLM call → tool calls → ... → final response.
func (l *Loop) Run(ctx context.Context, params RunParams) (string, error) {
	maxIter := l.MaxIterations
	if maxIter <= 0 {
		maxIter = DefaultMaxIterations
	}
	contextWindow := params.AgentConfig.Compaction.ContextWindow
	if contextWindow <= 0 {
		contextWindow = l.ContextWindow
	}
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindow
	}

	runID := fmt.Sprintf("run_%d", time.Now().UnixMilli())
	emitter := NewEventEmitter(runID, params.SessionMgr.SessionKey(), params.EventSink)

	// Load conversation history
	messages, err := params.SessionMgr.LoadTranscript()
	if err != nil {
		return "", fmt.Errorf("load transcript: %w", err)
	}

	// Convert attachments to LLM content: image -> ImageData; other types noted in text
	var images []llm.ImageData
	var otherParts []string
	for _, a := range params.Attachments {
		if a.Type == "image" {
			images = append(images, llm.ImageData{URL: a.URL, Base64: a.Base64, MIME: a.MIME})
		} else if a.Type != "" {
			if a.URL != "" {
				otherParts = append(otherParts, a.Type+": "+a.URL)
			} else {
				otherParts = append(otherParts, a.Type+" (inline)")
			}
		}
	}
	userText := params.UserMessage
	if len(otherParts) > 0 {
		if userText != "" {
			userText += "\n\n"
		}
		userText += "[Attached: " + strings.Join(otherParts, "; ") + "]"
	}
	var userMsg llm.Message
	if len(images) > 0 {
		userMsg = llm.UserMessageWithImages(userText, images)
	} else {
		userMsg = llm.UserMessage(userText)
	}
	messages = append(messages, userMsg)
	if err := params.SessionMgr.Append(userMsg); err != nil {
		slog.Warn("failed to append user message to transcript", "error", err)
	}

	// Resolve provider and model from agent config (agent.Provider + agent.Model)
	provider, model, provCfg, err := config.ResolveProviderForAgent(config.Get(), params.AgentConfig)
	if err != nil {
		return "", fmt.Errorf("resolve provider: %w", err)
	}

	// Build tool definitions filtered by policy
	toolDefs := l.Tools.ListToolDefs(l.Policy)

	// Build LLM params
	baseLLMParams := llm.ChatParams{
		Provider: provider,
		Model:    model,
		APIKey:   provCfg.APIKey,
		BaseURL:  provCfg.BaseURL,
		System:   params.SystemPrompt,
		Tools:    toolDefs,
	}

	var totalIn, totalOut int

	for i := 0; i < maxIter; i++ {
		select {
		case <-ctx.Done():
			return "", ErrAborted
		default:
		}

		emitter.Emit(EventTypeStreamStart, func(e *Event) {
			e.Text = fmt.Sprintf("iteration %d", i+1)
		})

		// Call LLM with fallback
		llmParams := baseLLMParams
		llmParams.Messages = messages

		result, err := l.callWithFallback(ctx, llmParams, params.AgentConfig, emitter)
		if err != nil {
			// Check for context overflow → try compaction
			var apiErr *llm.APIError
			if errors.As(err, &apiErr) && apiErr.IsContextOverflow() {
				slog.Info("context overflow, attempting compaction")
				emitter.Emit(EventTypeCompactStart)
				if compactErr := params.SessionMgr.DoCompact(ctx, l.resolveClient(provider), baseLLMParams, contextWindow); compactErr != nil {
					return "", fmt.Errorf("compaction failed: %w (original: %w)", compactErr, err)
				}
				emitter.Emit(EventTypeCompactEnd)
				// Reload messages after compaction
				messages, _ = params.SessionMgr.LoadTranscript()
				messages = append(messages, userMsg)
				continue
			}
			emitter.Emit(EventTypeError, func(e *Event) { e.Error = err.Error() })
			return "", err
		}

		// Track usage
		if result.Usage != nil {
			totalIn += result.Usage.InputTokens
			totalOut += result.Usage.OutputTokens
			params.SessionMgr.Store.UpdateUsage(params.SessionMgr.SessionKey(), result.Usage.InputTokens, result.Usage.OutputTokens)
		}

		// Persist assistant message
		if err := params.SessionMgr.Append(result.Message); err != nil {
			slog.Warn("failed to append assistant message", "error", err)
		}

		// Emit assistant text
		if result.Text != "" {
			emitter.Emit(EventTypeAssistant, func(e *Event) { e.Text = result.Text })
		}

		// No tool calls → done
		if len(result.ToolCalls) == 0 {
			emitter.Emit(EventTypeDone, func(e *Event) {
				e.TotalTokensIn = totalIn
				e.TotalTokensOut = totalOut
				e.Iterations = i + 1
			})
			return result.Text, nil
		}

		// Execute tool calls
		messages = append(messages, result.Message)
		for _, tc := range result.ToolCalls {
			emitter.Emit(EventTypeToolStart, func(e *Event) {
				e.ToolName = tc.Name
				e.ToolParams = tc.Arguments
			})

			toolResult, err := l.Tools.Execute(ctx, tc.Name, tc.Arguments)
			if err != nil {
				toolResult = fmt.Sprintf(`{"error": %q}`, err.Error())
			}

			emitter.Emit(EventTypeToolEnd, func(e *Event) {
				e.ToolName = tc.Name
				e.ToolResult = toolResult
			})
			if params.ToolSteps != nil {
				*params.ToolSteps = append(*params.ToolSteps, ToolStep{
					ToolName:   tc.Name,
					ToolParams: tc.Arguments,
					ToolResult: toolResult,
				})
			}

			toolMsg := llm.ToolResultMessage(tc.ID, toolResult)
			messages = append(messages, toolMsg)
			if err := params.SessionMgr.Append(toolMsg); err != nil {
				slog.Warn("failed to append tool result", "error", err)
			}
		}

		// Check if compaction needed after tool calls
		shouldCompact, _ := params.SessionMgr.ShouldCompact(contextWindow)
		if shouldCompact {
			emitter.Emit(EventTypeCompactStart)
			if err := params.SessionMgr.DoCompact(ctx, l.resolveClient(provider), baseLLMParams, contextWindow); err != nil {
				slog.Warn("post-iteration compaction failed", "error", err)
			} else {
				emitter.Emit(EventTypeCompactEnd)
				messages, _ = params.SessionMgr.LoadTranscript()
			}
		}
	}

	return "", ErrMaxIterations
}

// callWithFallback tries the primary model, then fallbacks on FailoverError.
func (l *Loop) callWithFallback(ctx context.Context, params llm.ChatParams, agentCfg *config.AgentConfig, emitter *EventEmitter) (*llm.StreamResult, error) {
	candidates := []string{agentCfg.Model}
	candidates = append(candidates, agentCfg.Fallbacks...)
	defaultProvider := agentCfg.Provider
	if defaultProvider == "" && strings.Contains(agentCfg.Model, "/") {
		if p, _, _, e := config.ResolveProvider(config.Get(), agentCfg.Model); e == nil {
			defaultProvider = p
		}
	}

	var lastErr error
	for _, modelRef := range candidates {
		provider, model, provCfg, err := config.ResolveProviderWithDefault(config.Get(), modelRef, defaultProvider)
		if err != nil {
			lastErr = err
			continue
		}

		p := params
		p.Provider = provider
		p.Model = model
		p.APIKey = provCfg.APIKey
		p.BaseURL = provCfg.BaseURL

		client := l.resolveClient(provider)
		stream, err := client.Chat(ctx, p)
		if err != nil {
			var apiErr *llm.APIError
			if errors.As(err, &apiErr) && (apiErr.IsRateLimit() || apiErr.IsAuth()) {
				slog.Warn("model failover", "model", modelRef, "error", err)
				lastErr = err
				continue
			}
			return nil, err
		}

		result, err := l.consumeWithEvents(ctx, stream, emitter)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	return nil, fmt.Errorf("all models failed, last error: %w", lastErr)
}

// consumeWithEvents reads the stream and emits text_delta events in real time.
func (l *Loop) consumeWithEvents(ctx context.Context, stream <-chan llm.StreamEvent, emitter *EventEmitter) (*llm.StreamResult, error) {
	result := &llm.StreamResult{}
	var textBuf []byte
	toolArgs := make(map[int]*[]byte)

	for event := range stream {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		switch event.Type {
		case "text_delta":
			textBuf = append(textBuf, event.Text...)
			emitter.Emit(EventTypeTextDelta, func(e *Event) { e.Text = event.Text })

		case "tool_call_delta":
			if _, ok := toolArgs[event.ToolCallIndex]; !ok {
				buf := []byte{}
				toolArgs[event.ToolCallIndex] = &buf
				result.ToolCalls = append(result.ToolCalls, llm.ToolCall{
					ID:   event.ToolCallID,
					Name: event.ToolCallName,
				})
			}
			*toolArgs[event.ToolCallIndex] = append(*toolArgs[event.ToolCallIndex], event.ToolCallArgs...)
			idx := event.ToolCallIndex
			if idx < len(result.ToolCalls) {
				if event.ToolCallID != "" {
					result.ToolCalls[idx].ID = event.ToolCallID
				}
				if event.ToolCallName != "" {
					result.ToolCalls[idx].Name = event.ToolCallName
				}
			}

		case "usage":
			if result.Usage == nil {
				result.Usage = &llm.Usage{}
			}
			if event.Usage != nil {
				result.Usage.InputTokens += event.Usage.InputTokens
				result.Usage.OutputTokens += event.Usage.OutputTokens
			}

		case "error":
			return nil, event.Error

		case "done":
			result.StopReason = event.Text
		}
	}

	result.Text = string(textBuf)
	for idx, args := range toolArgs {
		if idx < len(result.ToolCalls) {
			result.ToolCalls[idx].Arguments = string(*args)
		}
	}

	if len(result.ToolCalls) > 0 {
		result.Message = llm.Message{Role: llm.RoleAssistant, Content: result.Text, ToolCalls: result.ToolCalls}
	} else {
		result.Message = llm.Message{Role: llm.RoleAssistant, Content: result.Text}
	}

	return result, nil
}

// resolveClient picks the right LLM client based on provider config.
func (l *Loop) resolveClient(provider string) llm.Client {
	clientType := "openai"
	if cfg := config.Get(); cfg != nil {
		if provCfg, ok := cfg.Providers[provider]; ok {
			clientType = provCfg.ClientType(provider)
		}
	}
	if clientType == "anthropic" {
		if l.Anthropic == nil {
			l.Anthropic = llm.NewAnthropicClient()
		}
		return l.Anthropic
	}
	if l.OpenAI == nil {
		l.OpenAI = llm.NewOpenAIClient()
	}
	return l.OpenAI
}
