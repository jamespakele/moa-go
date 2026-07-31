package backend

// effectivePrompt returns the user prompt to send to the model. If the caller
// supplied an empty or whitespace-only prompt, it substitutes a neutral driver
// message so providers that reject empty user text blocks (e.g. Anthropic
// via OpenRouter) still receive a valid request.
func effectivePrompt(prompt string) string {
	const driver = "Proceed using the system prompt and the attached context files."
	if len(prompt) == 0 {
		return driver
	}
	for i := 0; i < len(prompt); i++ {
		if prompt[i] != ' ' && prompt[i] != '\t' && prompt[i] != '\n' && prompt[i] != '\r' {
			return prompt
		}
	}
	return driver
}
