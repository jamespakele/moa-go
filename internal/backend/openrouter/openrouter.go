// Package openrouter provides a native HTTP client for OpenRouter's
// /api/v1/chat/completions endpoint.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

// message is a single chat message.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// request is the JSON body for POST /api/v1/chat/completions.
type request struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *uint32   `json:"max_tokens,omitempty"`
}

// response is the OpenAI-compatible chat completion response.
type response struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Client is a thin native HTTP client for OpenRouter.
type Client struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// New creates an OpenRouter client from backend config, resolving the API key
// via the two-tier hierarchy:
//  1. explicit config api_key (non-empty)
//  2. OPENROUTER_API_KEY env var
//
// OpenRouter always requires authentication.
func New(baseURL *string, configKey *string, timeoutSecs uint64) (*Client, error) {
	key, err := ResolveAPIKey(configKey, os.Getenv)
	if err != nil {
		return nil, err
	}
	base := defaultBaseURL
	if baseURL != nil && strings.TrimSpace(*baseURL) != "" {
		base = *baseURL
	}
	return &Client{
		BaseURL: base,
		APIKey:  key,
		Timeout: time.Duration(timeoutSecs) * time.Second,
	}, nil
}

// Complete sends a chat request and returns the assistant's content.
func (c *Client) Complete(ctx context.Context, model, systemPrompt, prompt string, temp *float64, maxTok *uint32) (string, error) {
	var messages []message
	if systemPrompt != "" {
		messages = append(messages, message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, message{Role: "user", Content: prompt})

	body := request{
		Model:       model,
		Messages:    messages,
		Temperature: temp,
		MaxTokens:   maxTok,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OpenRouter request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	u, err := url.JoinPath(c.BaseURL, "/api/v1/chat/completions")
	if err != nil {
		return "", fmt.Errorf("failed to build OpenRouter URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create OpenRouter request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("HTTP-Referer", "https://github.com/jpakele/moa-go")
	req.Header.Set("X-Title", "moa-go")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("OpenRouter completion timed out after %v", c.Timeout)
		}
		return "", fmt.Errorf("OpenRouter completion failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		return "", fmt.Errorf("OpenRouter returned %s: %s", httpResp.Status, string(bodyBytes))
	}

	var resp response
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return "", fmt.Errorf("failed to decode OpenRouter response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("OpenRouter response contained no choices")
	}
	return resp.Choices[0].Message.Content, nil
}

// ResolveAPIKey resolves the OpenRouter API key from config or the
// OPENROUTER_API_KEY environment variable. The getenv injection keeps tests
// parallel-safe.
func ResolveAPIKey(configKey *string, getenv func(string) string) (string, error) {
	if configKey != nil && strings.TrimSpace(*configKey) != "" {
		return *configKey, nil
	}
	envKey := getenv("OPENROUTER_API_KEY")
	if strings.TrimSpace(envKey) == "" {
		if envKey == "" {
			return "", fmt.Errorf("OPENROUTER_API_KEY not set and no api_key in config (OpenRouter always requires auth)")
		}
		return "", fmt.Errorf("OPENROUTER_API_KEY is set but empty and no api_key in config (OpenRouter always requires auth)")
	}
	return envKey, nil
}
