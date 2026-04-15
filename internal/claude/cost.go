package claude

import (
	"fmt"
)

// Default pricing (Sonnet 4)
const (
	defaultInputRate  = 3.0  // $/M tokens
	defaultOutputRate = 15.0 // $/M tokens
)

type CostEstimate struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalUSD     float64 `json:"estimated_usd"`
	Method       string  `json:"method"` // "estimate"
}

// EstimateFromSession produces a rough cost estimate based on tool count and session duration.
//
// Model: each tool invocation ~ 2000 input tokens (context) + 500 output tokens.
// Session startup ~ 5000 input tokens for the initial prompt.
// Duration adds a small baseline for thinking/reasoning between tool calls.
func EstimateFromSession(ss *SessionStatus, inputRate, outputRate float64) *CostEstimate {
	if ss == nil {
		return nil
	}
	if inputRate <= 0 {
		inputRate = defaultInputRate
	}
	if outputRate <= 0 {
		outputRate = defaultOutputRate
	}

	toolCount := int64(ss.ToolCount)

	// Duration-based component: ~500 input tokens per minute of active session
	var durationMinutes float64
	if !ss.StartTime.IsZero() {
		dur := ss.Timestamp.Sub(ss.StartTime)
		if dur < 0 {
			dur = 0
		}
		durationMinutes = dur.Minutes()
	}

	inputTokens := int64(5000) + toolCount*2000 + int64(durationMinutes*500)
	outputTokens := toolCount * 500

	costInput := float64(inputTokens) / 1_000_000 * inputRate
	costOutput := float64(outputTokens) / 1_000_000 * outputRate

	return &CostEstimate{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalUSD:     costInput + costOutput,
		Method:       "estimate",
	}
}

// FormatCost returns a compact cost string like "~$0.42".
func FormatCost(c *CostEstimate) string {
	if c == nil {
		return "--"
	}
	if c.TotalUSD < 0.01 {
		return "~$0.00"
	}
	return fmt.Sprintf("~$%.2f", c.TotalUSD)
}
