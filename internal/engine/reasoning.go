package engine

import "strings"

const (
	reasoningOpen  = "⊗"
	reasoningClose = "</⊗>"
)

// SplitReasoning extracts a reasoning trace wrapped in ⊗ ... </⊗> tags from a
// model's raw output. It returns the reasoning trace (or nil if absent/empty) and
// the final answer with the tags removed. The behavior mirrors the Rust
// implementation exactly.
func SplitReasoning(raw string) (*string, string) {
	start := strings.Index(raw, reasoningOpen)
	if start < 0 {
		return nil, strings.TrimSpace(raw)
	}
	end := strings.Index(raw[start:], reasoningClose)
	if end < 0 {
		return nil, strings.TrimSpace(raw)
	}
	end += start

	reasoning := strings.TrimSpace(raw[start+len(reasoningOpen) : end])
	answer := strings.TrimSpace(raw[:start] + raw[end+len(reasoningClose):])
	if reasoning == "" {
		return nil, answer
	}
	return &reasoning, answer
}
