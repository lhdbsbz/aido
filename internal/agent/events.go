package agent

import "time"

// EventType constants
const (
	EventTypeStreamStart  = "stream_start"
	EventTypeTextDelta    = "text_delta"
	EventTypeToolStart    = "tool_start"
	EventTypeToolEnd      = "tool_end"
	EventTypeAssistant    = "assistant"
	EventTypeCompactStart = "compact_start"
	EventTypeCompactEnd   = "compact_end"
	EventTypeError        = "error"
	EventTypeDone         = "done"
)

// ToolStep represents one tool invocation (for API response and history).
type ToolStep struct {
	ToolName   string `json:"toolName"`
	ToolParams string `json:"toolParams"`
	ToolResult string `json:"toolResult"`
}

// Event is a structured event emitted by the Agent Loop.
// Gateway broadcasts these to connected clients/bridges.
type Event struct {
	Type       string    `json:"type"`
	RunID      string    `json:"runId"`
	SessionKey string    `json:"sessionKey"`
	Seq        int       `json:"seq"`
	Timestamp  time.Time `json:"timestamp"`

	// For text_delta
	Text string `json:"text,omitempty"`

	// For tool_start / tool_end
	ToolName   string `json:"toolName,omitempty"`
	ToolParams string `json:"toolParams,omitempty"`
	ToolResult string `json:"toolResult,omitempty"`

	// For error
	Error string `json:"error,omitempty"`

	// For done
	TotalTokensIn  int `json:"totalTokensIn,omitempty"`
	TotalTokensOut int `json:"totalTokensOut,omitempty"`
	Iterations     int `json:"iterations,omitempty"`
}

// EventSink receives events from the agent loop.
type EventSink func(Event)

// EventEmitter provides sequential event emission for a single run.
type EventEmitter struct {
	runID      string
	sessionKey string
	sink       EventSink
	seq        int
}

func NewEventEmitter(runID, sessionKey string, sink EventSink) *EventEmitter {
	return &EventEmitter{
		runID:      runID,
		sessionKey: sessionKey,
		sink:       sink,
	}
}

func (e *EventEmitter) Emit(eventType string, mutators ...func(*Event)) {
	if e.sink == nil {
		return
	}
	e.seq++
	evt := Event{
		Type:       eventType,
		RunID:      e.runID,
		SessionKey: e.sessionKey,
		Seq:        e.seq,
		Timestamp:  time.Now(),
	}
	for _, m := range mutators {
		m(&evt)
	}
	e.sink(evt)
}
