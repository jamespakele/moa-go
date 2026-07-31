// Package ollama provides a native HTTP client for Ollama's /api/chat endpoint.
package ollama

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

const defaultBaseURL = "http://localhost:11434"

// message is a single chat message.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// options holds the generation options sent in an Ollama request.
type options struct {
	Temperature *float64 `json:"temperature,omitempty"`
	NumPredict  *uint32  `json:"num_predict,omitempty"`
}

// request is the JSON body for POST /api/chat.
type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	Options  *options  `json:"options,omitempty"`
}

// response is the non-streaming JSON response from Ollama.
type response struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// Client is a thin native HTTP client for Ollama.
type Client struct {
	BaseURL string
	APIKey  string // empty means no Authorization header
	Timeout time.Duration
}

// New creates an Ollama client from backend config, resolving the API key via
// the three-tier hierarchy:
//  1. explicit config api_key (non-empty)
//  2. OLLAMA_API_KEY env var (for remote hosts)
//  3. no auth for localhost / 127.0.0.1 / ::1
func New(baseURL *string, configKey *string, timeoutSecs uint64) (*Client, error) {
	key, err := ResolveAPIKey(baseURL, configKey, os.Getenv)
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

	var opts *options
	if temp != nil || maxTok != nil {
		opts = &options{}
		if temp != nil {
			opts.Temperature = temp
		}
		if maxTok != nil {
			opts.NumPredict = maxTok
		}
	}

	body := request{
		Model:    model,
		Messages: messages,
		Stream:   false,
		Options:  opts,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	u, err := url.JoinPath(c.BaseURL, "/api/chat")
	if err != nil {
		return "", fmt.Errorf("failed to build Ollama URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("Ollama completion timed out after %v", c.Timeout)
		}
		return "", fmt.Errorf("Ollama completion failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		return "", fmt.Errorf("Ollama returned %s: %s", httpResp.Status, string(bodyBytes))
	}

	var resp response
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return "", fmt.Errorf("failed to decode Ollama response: %w", err)
	}
	return resp.Message.Content, nil
}

// ResolveAPIKey implements the three-tier Ollama API key resolution. It takes a
// getenv injection so tests can avoid mutating process-wide environment.
func ResolveAPIKey(baseURL *string, configKey *string, getenv func(string) string) (string, error) {
	if configKey != nil && strings.TrimSpace(*configKey) != "" {
		return *configKey, nil
	}

	isLocal := true
	if baseURL != nil && strings.TrimSpace(*baseURL) != "" {
		isLocal = isLocalOllamaHost(*baseURL)
	}
	if isLocal {
		return "", nil
	}

	envKey := getenv("OLLAMA_API_KEY")
	if strings.TrimSpace(envKey) == "" {
		if envKey == "" {
			return "", fmt.Errorf("OLLAMA_API_KEY not set and no api_key in config (remote Ollama requires auth)")
		}
		return "", fmt.Errorf("OLLAMA_API_KEY is set but empty and no api_key in config (remote Ollama requires auth)")
	}
	return envKey, nil
}

// isLocalOllamaHost extracts the host from a URL and returns true for
// localhost, 127.0.0.1, or ::1, including IPv6 bracket notation.
func isLocalOllamaHost(u string) bool {
	afterScheme := u
	if idx := strings.Index(u, "://"); idx != -1 {
		afterScheme = u[idx+len("://"):]
	}
	authority := afterScheme
	if idx := strings.Index(afterScheme, "/"); idx != -1 {
		authority = afterScheme[:idx]
	}

	var host string
	if strings.HasPrefix(authority, "[") {
		// IPv6 bracket form: [::1]:port — host is between brackets.
		host = strings.SplitN(strings.TrimPrefix(authority, "["), "]", 2)[0]
	} else {
		// host:port or just host.
		host = strings.SplitN(authority, ":", 2)[0]
	}

	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
