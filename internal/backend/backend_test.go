package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpakele/moa-go/internal/backend/ollama"
	"github.com/jpakele/moa-go/internal/backend/openrouter"
	"github.com/jpakele/moa-go/internal/callctx"
	"github.com/jpakele/moa-go/internal/config"
)

// --- effectivePrompt tests ---

func TestEffectivePromptPassesNonEmptyThrough(t *testing.T) {
	cases := []string{"Write the story.", "  spaced  "}
	for _, c := range cases {
		if got := effectivePrompt(c); got != c {
			t.Errorf("effectivePrompt(%q) = %q, want %q", c, got, c)
		}
	}
}

func TestEffectivePromptSubstitutesDriverWhenEmpty(t *testing.T) {
	driver := "Proceed using the system prompt and the attached context files."
	cases := []string{"", "   ", "\n\t"}
	for _, c := range cases {
		if got := effectivePrompt(c); got != driver {
			t.Errorf("effectivePrompt(%q) = %q, want driver", c, got)
		}
	}
	if got := effectivePrompt(""); got == "Write the story." {
		t.Error("empty prompt should not equal non-empty prompt")
	}
}

// --- API key resolution tests (injected env, no os.Getenv mutation) ---

func TestResolveAPIKeyOllamaConfigKeyTakesPrecedence(t *testing.T) {
	base := "http://remote.example.com:11434"
	key, err := ollama.ResolveAPIKey(&base, strPtr("config-key"), func(string) string { return "env-key" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "config-key" {
		t.Errorf("expected config-key, got %q", key)
	}
}

func TestResolveAPIKeyOllamaEnvVarFallbackRemote(t *testing.T) {
	base := "http://remote.example.com:11434"
	key, err := ollama.ResolveAPIKey(&base, nil, func(string) string { return "env-key" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key" {
		t.Errorf("expected env-key, got %q", key)
	}
}

func TestResolveAPIKeyOllamaLocalNeedsNoKey(t *testing.T) {
	cases := []string{
		"http://localhost:11434",
		"http://127.0.0.1:11434",
		"http://[::1]:11434",
	}
	for _, u := range cases {
		key, err := ollama.ResolveAPIKey(&u, nil, func(string) string { return "" })
		if err != nil {
			t.Errorf("%s: unexpected error: %v", u, err)
			continue
		}
		if key != "" {
			t.Errorf("%s: expected no key, got %q", u, key)
		}
	}
}

func TestResolveAPIKeyOllamaAbsentBaseURLIsLocal(t *testing.T) {
	key, err := ollama.ResolveAPIKey(nil, nil, func(string) string { return "" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Errorf("expected no key, got %q", key)
	}
}

func TestResolveAPIKeyOllamaRemoteNoKeyNoEnvErrors(t *testing.T) {
	base := "http://remote.example.com:11434"
	_, err := ollama.ResolveAPIKey(&base, nil, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OLLAMA_API_KEY not set") || !strings.Contains(err.Error(), "remote Ollama requires auth") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveAPIKeyOllamaRemoteEmptyEnvErrors(t *testing.T) {
	base := "http://remote.example.com:11434"
	_, err := ollama.ResolveAPIKey(&base, nil, func(string) string { return "   " })
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OLLAMA_API_KEY is set but empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveAPIKeyOllamaEmptyConfigKeyFallsThrough(t *testing.T) {
	base := "http://remote.example.com:11434"
	key, err := ollama.ResolveAPIKey(&base, strPtr(""), func(string) string { return "env-key" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key" {
		t.Errorf("expected env-key, got %q", key)
	}
}

// --- OpenRouter key resolution tests ---

func TestResolveAPIKeyOpenRouterConfigKey(t *testing.T) {
	key, err := openrouter.ResolveAPIKey(strPtr("config-key"), func(string) string { return "" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "config-key" {
		t.Errorf("expected config-key, got %q", key)
	}
}

func TestResolveAPIKeyOpenRouterConfigKeyTakesPrecedence(t *testing.T) {
	key, err := openrouter.ResolveAPIKey(strPtr("config-key"), func(string) string { return "env-key" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "config-key" {
		t.Errorf("expected config-key, got %q", key)
	}
}

func TestResolveAPIKeyOpenRouterEnvVarUsed(t *testing.T) {
	key, err := openrouter.ResolveAPIKey(nil, func(string) string { return "env-key" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key" {
		t.Errorf("expected env-key, got %q", key)
	}
}

func TestResolveAPIKeyOpenRouterEmptyConfigFallsThrough(t *testing.T) {
	key, err := openrouter.ResolveAPIKey(strPtr(""), func(string) string { return "env-key" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key" {
		t.Errorf("expected env-key, got %q", key)
	}
}

func TestResolveAPIKeyOpenRouterNoKeyErrors(t *testing.T) {
	_, err := openrouter.ResolveAPIKey(nil, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY not set") || !strings.Contains(err.Error(), "OpenRouter always requires auth") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveAPIKeyOpenRouterEmptyEnvErrors(t *testing.T) {
	_, err := openrouter.ResolveAPIKey(nil, func(string) string { return "   " })
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY is set but empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- buildPreamble tests ---

func TestBuildPreambleSkillDirReadsOnlySkillMd(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skill")
	refsDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Persona\nProduce the QA verdict."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refsDir, "big.md"), []byte("REFERENCE MATERIAL THAT MUST NOT APPEAR"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := callctx.CallContext{Skills: []string{skillDir}}
	preamble, err := buildPreamble(ctx, root)
	if err != nil {
		t.Fatalf("buildPreamble error: %v", err)
	}
	if !strings.Contains(preamble, "Produce the QA verdict.") {
		t.Errorf("preamble missing SKILL.md content: %q", preamble)
	}
	if strings.Contains(preamble, "REFERENCE MATERIAL THAT MUST NOT APPEAR") {
		t.Errorf("references/ content leaked into preamble: %q", preamble)
	}
}

func TestBuildPreambleConcatenatesSystemPromptAndSkill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("SKILL PERSONA BODY"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := callctx.CallContext{
		SystemPrompt: "AUTONOMOUS DIRECTIVE",
		Skills:       []string{skillDir},
	}
	preamble, err := buildPreamble(ctx, root)
	if err != nil {
		t.Fatalf("buildPreamble error: %v", err)
	}
	if !strings.Contains(preamble, "AUTONOMOUS DIRECTIVE") {
		t.Errorf("preamble missing system prompt: %q", preamble)
	}
	if !strings.Contains(preamble, "SKILL PERSONA BODY") {
		t.Errorf("preamble missing skill content: %q", preamble)
	}
}

func TestBuildPreambleSystemPromptSoftSkillFailure(t *testing.T) {
	root := t.TempDir()
	ctx := callctx.CallContext{
		SystemPrompt: "directive",
		Skills:       []string{"nonexistent-skill-xyzzy.md"},
	}
	preamble, err := buildPreamble(ctx, root)
	if err != nil {
		t.Fatalf("expected soft skill failure, got error: %v", err)
	}
	if preamble != "directive" {
		t.Errorf("expected 'directive', got %q", preamble)
	}
}

func TestBuildPreambleNoSystemPromptAllSkillsFailIsError(t *testing.T) {
	root := t.TempDir()
	ctx := callctx.CallContext{Skills: []string{"nonexistent-skill-xyzzy.md"}}
	_, err := buildPreamble(ctx, root)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "All skill files failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBuildPreambleAppendSystemPromptAfterSkill(t *testing.T) {
	root := t.TempDir()
	skillFile := filepath.Join(root, "skill.md")
	if err := os.WriteFile(skillFile, []byte("SKILL BODY"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := callctx.CallContext{
		SystemPrompt:       "DIRECTIVE",
		Skills:             []string{skillFile},
		AppendSystemPrompt: "APPENDIX",
	}
	preamble, err := buildPreamble(ctx, root)
	if err != nil {
		t.Fatalf("buildPreamble error: %v", err)
	}
	directiveIdx := strings.Index(preamble, "DIRECTIVE")
	skillIdx := strings.Index(preamble, "SKILL BODY")
	appendixIdx := strings.Index(preamble, "APPENDIX")
	if directiveIdx == -1 || skillIdx == -1 || appendixIdx == -1 {
		t.Fatalf("preamble missing expected sections: %q", preamble)
	}
	if !(directiveIdx < skillIdx && skillIdx < appendixIdx) {
		t.Errorf("wrong ordering in preamble: %q", preamble)
	}
}

// --- Mock provider completion tests ---

func TestOllamaBackendComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer ollama-key" {
			t.Errorf("unexpected Authorization header: %q", auth)
		}

		var body ollamaRequestLike
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if body.Model != "test-model" {
			t.Errorf("expected model test-model, got %q", body.Model)
		}
		if body.Stream {
			t.Error("expected stream=false")
		}
		if len(body.Messages) != 1 {
			t.Errorf("expected 1 message (empty preamble), got %d", len(body.Messages))
		}
		if body.Messages[0].Role != "user" || body.Messages[0].Content != "test prompt" {
			t.Errorf("unexpected user message: %+v", body.Messages[0])
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"ollama-hello"}}`)
	}))
	defer server.Close()

	baseURL := server.URL
	apiKey := "ollama-key"
	cfg := config.BackendConfig{
		BaseURL:     &baseURL,
		APIKey:      &apiKey,
		TimeoutSecs: 10,
	}
	b := New(cfg)
	result, err := b.Complete(context.Background(), "ollama", "test-model", "test prompt", "off", callctx.CallContext{})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if result != "ollama-hello" {
		t.Errorf("expected ollama-hello, got %q", result)
	}
}

func TestOpenRouterBackendComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer openrouter-key" {
			t.Errorf("unexpected Authorization header: %q", auth)
		}
		if ref := r.Header.Get("HTTP-Referer"); ref != "https://github.com/jpakele/moa-go" {
			t.Errorf("unexpected HTTP-Referer: %q", ref)
		}
		if title := r.Header.Get("X-Title"); title != "moa-go" {
			t.Errorf("unexpected X-Title: %q", title)
		}

		var body openrouterRequestLike
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if body.Model != "anthropic/claude-3.5-sonnet" {
			t.Errorf("expected model with slash, got %q", body.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"choices":[{"message":{"role":"assistant","content":"openrouter-hello"}}]}`)
	}))
	defer server.Close()

	baseURL := server.URL
	apiKey := "openrouter-key"
	cfg := config.BackendConfig{
		BaseURL:     &baseURL,
		APIKey:      &apiKey,
		TimeoutSecs: 10,
	}
	b := New(cfg)
	result, err := b.Complete(context.Background(), "openrouter", "anthropic/claude-3.5-sonnet", "test prompt", "off", callctx.CallContext{})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if result != "openrouter-hello" {
		t.Errorf("expected openrouter-hello, got %q", result)
	}
}

func TestBackendUnknownProvider(t *testing.T) {
	cfg := config.BackendConfig{TimeoutSecs: 10}
	b := New(cfg)
	_, err := b.Complete(context.Background(), "unknown-provider", "model", "test", "off", callctx.CallContext{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown provider: unknown-provider") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBackendContextFileNotFound(t *testing.T) {
	cfg := config.BackendConfig{TimeoutSecs: 10}
	b := New(cfg)
	_, err := b.Complete(context.Background(), "ollama", "model", "test", "off", callctx.CallContext{
		Files: []string{"nonexistent-context-file-xyzzy.md"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Context file error") || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBackendSkillFileNotFound(t *testing.T) {
	cfg := config.BackendConfig{TimeoutSecs: 10}
	b := New(cfg)
	_, err := b.Complete(context.Background(), "ollama", "model", "test", "off", callctx.CallContext{
		Skills: []string{"nonexistent-skill-file-xyzzy.md"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "All skill files failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOllamaBackendCompleteUsesDriverWhenPromptEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ollamaRequestLike
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(body.Messages))
		}
		driver := "Proceed using the system prompt and the attached context files."
		if body.Messages[0].Content != driver {
			t.Errorf("expected driver prompt, got %q", body.Messages[0].Content)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"ok"}}`)
	}))
	defer server.Close()

	baseURL := server.URL
	cfg := config.BackendConfig{BaseURL: &baseURL, TimeoutSecs: 10}
	b := New(cfg)
	if _, err := b.Complete(context.Background(), "ollama", "m", "", "off", callctx.CallContext{}); err != nil {
		t.Fatalf("Complete error: %v", err)
	}
}

func TestOllamaBackendCompleteWithSystemPromptAndThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ollamaRequestLike
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(body.Messages))
		}
		if body.Messages[0].Role != "system" {
			t.Errorf("expected system message first, got %s", body.Messages[0].Role)
		}
		if !strings.Contains(body.Messages[0].Content, "be nice") {
			t.Errorf("system message missing prompt: %q", body.Messages[0].Content)
		}
		if !strings.Contains(body.Messages[0].Content, "Show your reasoning in ⊗ tags") {
			t.Errorf("system message missing thinking instruction: %q", body.Messages[0].Content)
		}
		if body.Messages[1].Role != "user" {
			t.Errorf("expected user message second, got %s", body.Messages[1].Role)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"ok"}}`)
	}))
	defer server.Close()

	baseURL := server.URL
	cfg := config.BackendConfig{BaseURL: &baseURL, TimeoutSecs: 10}
	b := New(cfg)
	ctx := callctx.CallContext{SystemPrompt: "be nice"}
	if _, err := b.Complete(context.Background(), "ollama", "m", "hello", "low", ctx); err != nil {
		t.Fatalf("Complete error: %v", err)
	}
}

func TestOllamaBackendCompletePassesTemperatureAndMaxTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ollamaOptionsLike
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Options == nil {
			t.Fatal("expected options")
		}
		if body.Options.Temperature == nil || *body.Options.Temperature != 0.3 {
			t.Errorf("expected temperature 0.3, got %v", body.Options.Temperature)
		}
		if body.Options.NumPredict == nil || *body.Options.NumPredict != 512 {
			t.Errorf("expected num_predict 512, got %v", body.Options.NumPredict)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"ok"}}`)
	}))
	defer server.Close()

	baseURL := server.URL
	cfg := config.BackendConfig{BaseURL: &baseURL, TimeoutSecs: 10}
	b := New(cfg)
	temp := 0.3
	tok := uint32(512)
	cc := callctx.CallContext{Temperature: &temp, MaxTokens: &tok}
	if _, err := b.Complete(context.Background(), "ollama", "m", "hello", "off", cc); err != nil {
		t.Fatalf("Complete error: %v", err)
	}
}

func TestOpenRouterBackendCompleteWithSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body openrouterRequestLike
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(body.Messages))
		}
		if body.Messages[0].Role != "system" || !strings.Contains(body.Messages[0].Content, "sys") {
			t.Errorf("unexpected system message: %+v", body.Messages[0])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()

	baseURL := server.URL
	apiKey := "key"
	cfg := config.BackendConfig{BaseURL: &baseURL, APIKey: &apiKey, TimeoutSecs: 10}
	b := New(cfg)
	if _, err := b.Complete(context.Background(), "openrouter", "m", "hello", "off", callctx.CallContext{SystemPrompt: "sys"}); err != nil {
		t.Fatalf("Complete error: %v", err)
	}
}

func TestOpenRouterBackendCompleteOmitsNilOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, hasTemp := body["temperature"]; hasTemp {
			t.Error("expected temperature omitted")
		}
		if _, hasMax := body["max_tokens"]; hasMax {
			t.Error("expected max_tokens omitted")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()

	baseURL := server.URL
	apiKey := "key"
	cfg := config.BackendConfig{BaseURL: &baseURL, APIKey: &apiKey, TimeoutSecs: 10}
	b := New(cfg)
	if _, err := b.Complete(context.Background(), "openrouter", "m", "hello", "off", callctx.CallContext{}); err != nil {
		t.Fatalf("Complete error: %v", err)
	}
}

// --- helpers ---

type ollamaOptionsLike struct {
	Options *struct {
		Temperature *float64 `json:"temperature"`
		NumPredict  *uint32  `json:"num_predict"`
	} `json:"options"`
}

func strPtr(s string) *string { return &s }

// Minimal request-shape mirrors for the mock server assertions.
type ollamaRequestLike struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Stream bool `json:"stream"`
}

type openrouterRequestLike struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}
