package config

import (
	"os"
	"path/filepath"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestLoadFixture(t *testing.T) {
	fixture := `[aggregator]
provider = "ollama"
model = "glm-5.2:cloud"

[[reference]]
provider = "ollama"
model = "nemotron-3-super:cloud"
label = "nemotron"

[[reference]]
provider = "openrouter"
model = "anthropic/claude-3.5-sonnet"
label = "openrouter-claude"

[backend]
backend = "native"
timeout_secs = 120
reference_thinking = "low"

[synthesis]
temperature = 0.4
max_tokens = 4096
output_dir = "./output"
max_rounds = 0
bmad_compatible = false
bmad_config_path = "_bmad/bmm/config.yaml"

[search]
provider = "tavily"
api_key = "tvly-test"
max_queries = 3

[verifier]
enabled = true
provider = "ollama"
model = "glm-5.2:cloud"
temperature = 0.1
max_tokens = 2048
`
	dir := t.TempDir()
	path := filepath.Join(dir, "moa.toml")
	if err := os.WriteFile(path, []byte(fixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(fixture) failed: %v", err)
	}
	if cfg.Aggregator.Provider != "ollama" || cfg.Aggregator.Model != "glm-5.2:cloud" {
		t.Errorf("aggregator mismatch: got %s/%s", cfg.Aggregator.Provider, cfg.Aggregator.Model)
	}
	if len(cfg.Reference) != 2 {
		t.Fatalf("expected 2 reference models, got %d", len(cfg.Reference))
	}
	if cfg.Reference[0].Label == nil || *cfg.Reference[0].Label != "nemotron" {
		t.Errorf("reference label missing/wrong: %+v", cfg.Reference[0])
	}
	if cfg.Reference[1].Provider != "openrouter" || cfg.Reference[1].Model != "anthropic/claude-3.5-sonnet" {
		t.Errorf("openrouter reference wrong: %+v", cfg.Reference[1])
	}
	if cfg.Backend.Backend != "native" || cfg.Backend.TimeoutSecs != 120 {
		t.Errorf("unexpected backend defaults: %+v", cfg.Backend)
	}
	if cfg.Synthesis.MaxTokens != 4096 || cfg.Synthesis.Temperature != 0.4 {
		t.Errorf("unexpected synthesis defaults: %+v", cfg.Synthesis)
	}
	if cfg.Search.Provider != "tavily" || cfg.Search.APIKey != "tvly-test" || cfg.Search.MaxQueries != 3 {
		t.Errorf("unexpected search values: %+v", cfg.Search)
	}
	if !cfg.Verifier.Enabled || cfg.Verifier.Provider != "ollama" || cfg.Verifier.Model != "glm-5.2:cloud" {
		t.Errorf("unexpected verifier values: %+v", cfg.Verifier)
	}
	if cfg.Verifier.Temperature == nil || *cfg.Verifier.Temperature != 0.1 || cfg.Verifier.MaxTokens == nil || *cfg.Verifier.MaxTokens != 2048 {
		t.Errorf("unexpected verifier overrides: %+v", cfg.Verifier)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Aggregator.Provider != "ollama" || cfg.Aggregator.Model != "glm-5.2:cloud" {
		t.Errorf("default aggregator mismatch: %s/%s", cfg.Aggregator.Provider, cfg.Aggregator.Model)
	}
	if len(cfg.Reference) != 3 {
		t.Errorf("expected 3 default references, got %d", len(cfg.Reference))
	}
	if cfg.Backend.Backend != "native" || cfg.Backend.TimeoutSecs != 120 {
		t.Errorf("unexpected backend defaults: %+v", cfg.Backend)
	}
	if cfg.Synthesis.Temperature != 0.4 || cfg.Synthesis.MaxTokens != 4096 {
		t.Errorf("unexpected synthesis defaults: %+v", cfg.Synthesis)
	}
}

func validConfig() MoaConfig {
	cfg := DefaultConfig()
	cfg.Reference = []AgentSlot{
		{Provider: "ollama", Model: "nemotron-3-super:cloud"},
	}
	return cfg
}

func TestValidateAggregatorErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*MoaConfig)
		wantErr string
	}{
		{
			name: "empty aggregator provider",
			mutate: func(c *MoaConfig) { c.Aggregator.Provider = "" },
			wantErr: "Aggregator provider is empty",
		},
		{
			name: "empty aggregator model",
			mutate: func(c *MoaConfig) { c.Aggregator.Model = "" },
			wantErr: "Aggregator model is empty",
		},
		{
			name: "aggregator temperature out of range",
			mutate: func(c *MoaConfig) { c.Aggregator.Temperature = ptr(2.1) },
			wantErr: "Aggregator temperature 2.10 is out of range",
		},
		{
			name: "aggregator max_tokens zero",
			mutate: func(c *MoaConfig) { c.Aggregator.MaxTokens = ptr(uint32(0)) },
			wantErr: "Aggregator max_tokens must be greater than 0",
		},
		{
			name: "pi-dev backend rejected",
			mutate: func(c *MoaConfig) { c.Backend.Backend = "pi-dev" },
			wantErr: "Unknown backend: 'pi-dev'",
		},
		{
			name: "unknown backend rejected",
			mutate: func(c *MoaConfig) { c.Backend.Backend = "unknown" },
			wantErr: "Unknown backend: 'unknown'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateReferenceErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*MoaConfig)
		wantErr string
	}{
		{
			name: "empty reference provider",
			mutate: func(c *MoaConfig) { c.Reference[0].Provider = "" },
			wantErr: "Reference 1 has empty provider",
		},
		{
			name: "empty reference model",
			mutate: func(c *MoaConfig) { c.Reference[0].Model = "" },
			wantErr: "Reference 1 has empty model",
		},
		{
			name: "shell-dangerous model char",
			mutate: func(c *MoaConfig) { c.Reference[0].Model = "model;rm -rf" },
			wantErr: "contains shell-dangerous characters",
		},
		{
			name: "reference temperature out of range",
			mutate: func(c *MoaConfig) { c.Reference[0].Temperature = ptr(-0.1) },
			wantErr: "Reference 1 temperature -0.10 is out of range",
		},
		{
			name: "reference max_tokens zero",
			mutate: func(c *MoaConfig) { c.Reference[0].MaxTokens = ptr(uint32(0)) },
			wantErr: "Reference 1 max_tokens must be greater than 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFindProjectRoot(t *testing.T) {
	root := FindProjectRoot()
	if _, err := os.Stat(filepath.Join(root, "moa.toml")); err != nil {
		t.Errorf("FindProjectRoot() = %s, no moa.toml found: %v", root, err)
	}
}

func TestAddRemoveReference(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.AddReference("openai/gpt-4o", ptr("gpt4o")); err != nil {
		t.Fatalf("AddReference failed: %v", err)
	}
	if len(cfg.Reference) != 4 || cfg.Reference[3].Provider != "openai" || cfg.Reference[3].Model != "gpt-4o" {
		t.Errorf("AddReference produced wrong slot: %+v", cfg.Reference[3])
	}
	if !cfg.RemoveReference("openai/gpt-4o") {
		t.Error("RemoveReference should return true")
	}
	if len(cfg.Reference) != 3 {
		t.Errorf("expected 3 references after remove, got %d", len(cfg.Reference))
	}
}

func TestAddReferenceInvalidFormat(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.AddReference("invalid-format", nil); err == nil || !contains(err.Error(), "provider/model must be in format") {
		t.Errorf("expected format error, got %v", err)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || len(sub) == 0 || findSubstr(s, sub)) }

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
