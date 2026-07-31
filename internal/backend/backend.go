// Package backend provides the LLM backend abstraction.
package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpakele/moa-go/internal/backend/ollama"
	"github.com/jpakele/moa-go/internal/backend/openrouter"
	"github.com/jpakele/moa-go/internal/callctx"
	"github.com/jpakele/moa-go/internal/config"
)

// Backend is the object-safe Go replacement for Rust's MoaBackend trait.
// The Go interface is naturally object-safe, so no generic helper is needed.
type Backend interface {
	Complete(ctx context.Context, provider, model, prompt, thinking string, cc callctx.CallContext) (string, error)
}

// composite implements Backend by dispatching to native provider clients.
type composite struct {
	cfg config.BackendConfig
}

// New returns a Backend that routes provider strings to native HTTP clients.
// Provider-specific clients are created per-call so base_url/api_key can be honored.
func New(cfg config.BackendConfig) Backend {
	return &composite{cfg: cfg}
}

func (c *composite) Complete(ctx context.Context, provider, model, prompt, thinking string, cc callctx.CallContext) (string, error) {
	projectRoot := config.FindProjectRoot()
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve project root: %w", err)
	}
	rootResolved, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		rootResolved = absRoot
	}

	preamble, err := buildPreamble(cc, rootResolved)
	if err != nil {
		return "", err
	}

	if thinking != "" && thinking != "off" {
		preamble += "\n\nShow your reasoning in ⊗ tags before your final answer."
	}

	if len(cc.Files) > 0 {
		var docs []string
		for _, filePath := range cc.Files {
			verifiedPath, err := canonicalizeAndVerifyPath(filePath, rootResolved)
			if err != nil {
				return "", fmt.Errorf("Context file error: %s", err)
			}
			content, err := os.ReadFile(verifiedPath)
			if err != nil {
				return "", fmt.Errorf("Failed to read context file %s: %w", filePath, err)
			}
			docs = append(docs, string(content))
		}
		if len(docs) > 0 {
			preamble += "\n\n## Context Files\n"
			for i, doc := range docs {
				preamble += fmt.Sprintf("\n### File %d\n```\n%s\n```\n", i+1, doc)
			}
		}
	}

	promptToSend := effectivePrompt(prompt)

	temperature := cc.Temperature
	maxTokens := cc.MaxTokens

	switch strings.ToLower(provider) {
	case "ollama":
		client, err := ollama.New(c.cfg.BaseURL, c.cfg.APIKey, c.cfg.TimeoutSecs)
		if err != nil {
			return "", err
		}
		return client.Complete(ctx, model, preamble, promptToSend, temperature, maxTokens)
	case "openrouter":
		client, err := openrouter.New(c.cfg.BaseURL, c.cfg.APIKey, c.cfg.TimeoutSecs)
		if err != nil {
			return "", err
		}
		return client.Complete(ctx, model, preamble, promptToSend, temperature, maxTokens)
	default:
		return "", fmt.Errorf("unknown provider: %s", provider)
	}
}
