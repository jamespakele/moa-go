// Package callctx holds the per-call context forwarded to each model.
package callctx

// CallContext is the Go equivalent of Rust's PiContext.
// Zero value matches PiContext::default(): no skills/files/prompts, no
// temperature/max_tokens overrides, no tools.
type CallContext struct {
	Skills             []string
	Files              []string
	SystemPrompt       string
	AppendSystemPrompt string
	Temperature        *float64
	MaxTokens          *uint32
	Tools              []string
}
