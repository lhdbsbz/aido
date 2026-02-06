package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UsageRecord tracks a single LLM call's token usage.
type UsageRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"inputTokens"`
	OutputTokens int       `json:"outputTokens"`
	SessionKey   string    `json:"sessionKey,omitempty"`
}

// UsageTracker records and aggregates token usage.
type UsageTracker struct {
	mu      sync.Mutex
	logPath string
	totals  UsageTotals
}

type UsageTotals struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	EstCostUSD   float64 `json:"estCostUSD"`
}

func NewUsageTracker(dataDir string) *UsageTracker {
	os.MkdirAll(filepath.Join(dataDir, "usage"), 0755)
	return &UsageTracker{
		logPath: filepath.Join(dataDir, "usage", "usage.jsonl"),
	}
}

// Record logs a usage record and updates totals.
func (t *UsageTracker) Record(rec UsageRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.totals.InputTokens += rec.InputTokens
	t.totals.OutputTokens += rec.OutputTokens
	t.totals.EstCostUSD += estimateCost(rec.Provider, rec.Model, rec.InputTokens, rec.OutputTokens)

	// Append to JSONL log
	f, err := os.OpenFile(t.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	data, _ := json.Marshal(rec)
	data = append(data, '\n')
	f.Write(data)
}

// Totals returns aggregated usage.
func (t *UsageTracker) Totals() UsageTotals {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totals
}

// Status returns a human-readable usage summary.
func (t *UsageTracker) Status() string {
	totals := t.Totals()
	return fmt.Sprintf("Tokens: %d in / %d out | Est. cost: $%.4f",
		totals.InputTokens, totals.OutputTokens, totals.EstCostUSD)
}

// estimateCost gives a rough USD cost estimate.
func estimateCost(provider, model string, input, output int) float64 {
	// Per-1M token pricing (approximate, 2025 rates)
	var inPer1M, outPer1M float64
	switch {
	case provider == "anthropic" && contains(model, "opus"):
		inPer1M, outPer1M = 15.0, 75.0
	case provider == "anthropic" && contains(model, "sonnet"):
		inPer1M, outPer1M = 3.0, 15.0
	case provider == "anthropic" && contains(model, "haiku"):
		inPer1M, outPer1M = 0.25, 1.25
	case provider == "openai" && contains(model, "gpt-4o"):
		inPer1M, outPer1M = 2.5, 10.0
	case provider == "openai" && contains(model, "gpt-4"):
		inPer1M, outPer1M = 30.0, 60.0
	case contains(model, "deepseek"):
		inPer1M, outPer1M = 0.14, 0.28
	default:
		inPer1M, outPer1M = 1.0, 3.0 // conservative default
	}
	return (float64(input)/1_000_000)*inPer1M + (float64(output)/1_000_000)*outPer1M
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
