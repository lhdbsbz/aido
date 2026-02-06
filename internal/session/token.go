package session

import "github.com/lhdbsbz/aido/internal/llm"

// EstimateTokens gives a rough token count for a string.
// English: ~4 chars/token, Chinese: ~2 chars/token.
// This is intentionally simple — exact counting needs tiktoken which is heavy.
// We use a 1.2x safety margin in compaction to compensate.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}

	chars := 0
	cjk := 0
	for _, r := range text {
		chars++
		if r >= 0x4E00 && r <= 0x9FFF {
			cjk++
		}
	}

	// CJK characters are roughly 1 token per 1-2 chars
	// ASCII is roughly 1 token per 4 chars
	ascii := chars - cjk
	return (ascii / 4) + (cjk * 2 / 3) + 1
}

// EstimateMessagesTokens estimates total tokens for a slice of messages.
func EstimateMessagesTokens(messages []llm.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateTokens(msg.Content)
		total += 4 // role + formatting overhead per message
		for _, tc := range msg.ToolCalls {
			total += EstimateTokens(tc.Name) + EstimateTokens(tc.Arguments)
		}
	}
	return total
}
