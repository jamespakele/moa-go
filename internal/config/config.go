// Package config loads and validates moa.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// MoaConfig is the top-level configuration loaded from moa.toml.
type MoaConfig struct {
	Aggregator AgentSlot        `toml:"aggregator"`
	Reference  []AgentSlot      `toml:"reference"`
	Backend    BackendConfig    `toml:"backend"`
	Synthesis  SynthesisConfig  `toml:"synthesis"`
	Search     SearchConfig     `toml:"search"`
	Verifier   VerifierConfig   `toml:"verifier"`
}

// AgentSlot is a single model slot (aggregator or reference).
type AgentSlot struct {
	Provider    string   `toml:"provider"`
	Model       string   `toml:"model"`
	Label       *string  `toml:"label,omitempty"`
	Skill       *string  `toml:"skill,omitempty"`
	Temperature *float64 `toml:"temperature,omitempty"`
	MaxTokens   *uint32  `toml:"max_tokens,omitempty"`
}

// BackendConfig holds the native HTTP backend settings.
type BackendConfig struct {
	Backend           string  `toml:"backend"`
	APIKey            *string `toml:"api_key,omitempty"`
	BaseURL           *string `toml:"base_url,omitempty"`
	TimeoutSecs       uint64  `toml:"timeout_secs"`
	ReferenceThinking string  `toml:"reference_thinking"`
}

// SynthesisConfig holds aggregator-level synthesis parameters.
type SynthesisConfig struct {
	Temperature    float64 `toml:"temperature"`
	MaxTokens      uint32  `toml:"max_tokens"`
	OutputDir      string  `toml:"output_dir"`
	MaxRounds      uint32  `toml:"max_rounds"`
	BMadCompatible bool    `toml:"bmad_compatible"`
	BMadConfigPath string  `toml:"bmad_config_path"`
}

// SearchConfig configures the web-search backend used by the verifier.
type SearchConfig struct {
	Provider   string `toml:"provider"`
	APIKey     string `toml:"api_key"`
	MaxQueries int    `toml:"max_queries"`
}

// VerifierConfig gates the verify+revise pass and overrides the aggregator
// provider/model/temperature/max_tokens when set.
type VerifierConfig struct {
	Enabled     bool     `toml:"enabled"`
	Provider    string   `toml:"provider"`
	Model       string   `toml:"model"`
	Temperature *float64 `toml:"temperature,omitempty"`
	MaxTokens   *uint32  `toml:"max_tokens,omitempty"`
}

// ConfigError represents a failure during config loading/saving.
type ConfigError struct {
	Op  string
	Err error
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *ConfigError) Unwrap() error { return e.Err }

// FindProjectRoot walks up from the current directory looking for moa.toml.
// If not found it returns the current directory and prints a warning.
func FindProjectRoot() string {
	start, err := filepath.Abs(".")
	if err != nil {
		start = "."
	}
	current := start
	for {
		candidate := filepath.Join(current, "moa.toml")
		if _, err := os.Stat(candidate); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			fmt.Fprintln(os.Stderr, "[moa] Warning: no moa.toml found in directory tree, path verification may be less strict")
			return start
		}
		current = parent
	}
}

// DefaultConfig returns the same default configuration as the Rust binary.
func DefaultConfig() MoaConfig {
	temp04 := 0.4
	tok4096 := uint32(4096)
	temp02 := 0.2
	return MoaConfig{
		Aggregator: AgentSlot{
			Provider: "ollama",
			Model:    "glm-5.2:cloud",
			Label:    nil,
			Skill:    nil,
			Temperature: &temp04,
			MaxTokens:   &tok4096,
		},
		Reference: []AgentSlot{
			{Provider: "ollama", Model: "nemotron-3-super:cloud", Label: func() *string { s := "nemotron"; return &s }(), Temperature: nil, MaxTokens: nil},
			{Provider: "ollama", Model: "qwen3.5:397b-cloud", Label: func() *string { s := "qwen35"; return &s }(), Temperature: nil, MaxTokens: nil},
			{Provider: "ollama", Model: "deepseek-v4-flash:cloud", Label: func() *string { s := "deepseek-v4-flash"; return &s }(), Temperature: nil, MaxTokens: nil},
		},
		Backend: BackendConfig{
			Backend:           "native",
			APIKey:            nil,
			BaseURL:           nil,
			TimeoutSecs:       120,
			ReferenceThinking: "",
		},
		Synthesis: SynthesisConfig{
			Temperature:    0.4,
			MaxTokens:      4096,
			OutputDir:      "./output",
			MaxRounds:      0,
			BMadCompatible: false,
			BMadConfigPath: "_bmad/bmm/config.yaml",
		},
		Search: SearchConfig{
			Provider:   "tavily",
			APIKey:     "",
			MaxQueries: 5,
		},
		Verifier: VerifierConfig{
			Enabled:     false,
			Provider:    "",
			Model:       "",
			Temperature: &temp02,
			MaxTokens:   &tok4096,
		},
	}
}

// Load reads and decodes a moa.toml file, applying defaults for missing fields.
func Load(path string) (MoaConfig, error) {
	var cfg MoaConfig
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, &ConfigError{Op: "load", Err: fmt.Errorf("config file not found: %s", path)}
		}
		return cfg, &ConfigError{Op: "load", Err: fmt.Errorf("failed to read config: %w", err)}
	}
	if err := toml.Unmarshal(content, &cfg); err != nil {
		return cfg, &ConfigError{Op: "load", Err: fmt.Errorf("parse error: %w", err)}
	}
	applyDefaults(&cfg)
	return cfg, nil
}

// Save writes the configuration to a TOML file.
func Save(cfg MoaConfig, path string) error {
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	if err := enc.Encode(cfg); err != nil {
		return &ConfigError{Op: "save", Err: fmt.Errorf("encode error: %w", err)}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return &ConfigError{Op: "save", Err: fmt.Errorf("write error: %w", err)}
	}
	return nil
}

// applyDefaults fills in sensible defaults for fields that were not supplied.
func applyDefaults(cfg *MoaConfig) {
	if cfg.Backend.Backend == "" {
		cfg.Backend.Backend = "native"
	}

	if cfg.Backend.TimeoutSecs == 0 {
		cfg.Backend.TimeoutSecs = 120
	}
	if cfg.Backend.ReferenceThinking == "" {
		cfg.Backend.ReferenceThinking = "low"
	}
	if cfg.Synthesis.OutputDir == "" {
		cfg.Synthesis.OutputDir = "./output"
	}
	if cfg.Synthesis.MaxTokens == 0 {
		cfg.Synthesis.MaxTokens = 4096
	}
	// Note: Rust default_config sets temperature 0.4. Since we cannot
	// distinguish an explicit 0.0 from an absent value, we only apply the
	// default when the value is exactly 0.0. This matches the common case.
	if cfg.Synthesis.Temperature == 0.0 {
		cfg.Synthesis.Temperature = 0.4
	}
	if cfg.Synthesis.BMadConfigPath == "" {
		cfg.Synthesis.BMadConfigPath = "_bmad/bmm/config.yaml"
	}
	if cfg.Search.Provider == "" {
		cfg.Search.Provider = "tavily"
	}
	if cfg.Search.MaxQueries == 0 {
		cfg.Search.MaxQueries = 5
	}
	if !cfg.Verifier.Enabled {
		// Default temperature/max_tokens for verifier when not explicitly set.
		if cfg.Verifier.Temperature == nil {
			t := 0.2
			cfg.Verifier.Temperature = &t
		}
		if cfg.Verifier.MaxTokens == nil {
			m := uint32(4096)
			cfg.Verifier.MaxTokens = &m
		}
	}
}

// AddReference parses a "provider/model" string and appends a new slot.
func (cfg *MoaConfig) AddReference(providerModel string, label *string) error {
	parts := strings.SplitN(providerModel, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("provider/model must be in format <provider>/<model>, got: %s", providerModel)
	}
	cfg.Reference = append(cfg.Reference, AgentSlot{
		Provider: parts[0],
		Model:    parts[1],
		Label:    label,
	})
	return nil
}

// RemoveReference drops the first reference whose provider/model match.
func (cfg *MoaConfig) RemoveReference(providerModel string) bool {
	before := len(cfg.Reference)
	cfg.Reference = filter(cfg.Reference, func(a AgentSlot) bool {
		return fmt.Sprintf("%s/%s", a.Provider, a.Model) != providerModel
	})
	return len(cfg.Reference) != before
}

func filter(in []AgentSlot, keep func(AgentSlot) bool) []AgentSlot {
	out := make([]AgentSlot, 0, len(in))
	for _, a := range in {
		if keep(a) {
			out = append(out, a)
		}
	}
	return out
}

// Validate returns a non-nil error if the configuration cannot run.
func (cfg *MoaConfig) Validate() error {
	if cfg.Backend.Backend != "native" {
		return fmt.Errorf("Unknown backend: '%s'. Must be 'native'", cfg.Backend.Backend)
	}

	if cfg.Aggregator.Provider == "" {
		return fmt.Errorf("Aggregator provider is empty")
	}
	if cfg.Aggregator.Model == "" {
		return fmt.Errorf("Aggregator model is empty")
	}
	if cfg.Aggregator.Temperature != nil && (*cfg.Aggregator.Temperature < 0.0 || *cfg.Aggregator.Temperature > 2.0) {
		return fmt.Errorf("Aggregator temperature %.2f is out of range (0.0–2.0)", *cfg.Aggregator.Temperature)
	}
	if cfg.Aggregator.MaxTokens != nil && *cfg.Aggregator.MaxTokens == 0 {
		return fmt.Errorf("Aggregator max_tokens must be greater than 0")
	}

	for i, refAgent := range cfg.Reference {
		if refAgent.Provider == "" {
			return fmt.Errorf("Reference %d has empty provider", i+1)
		}
		if refAgent.Model == "" {
			return fmt.Errorf("Reference %d has empty model", i+1)
		}
		dangerous := []rune(";|&$`><(){}\n\r")
		for _, r := range refAgent.Model {
			for _, d := range dangerous {
				if r == d {
					return fmt.Errorf("Reference %d model '%s' contains shell-dangerous characters", i+1, refAgent.Model)
				}
			}
		}
		if refAgent.Temperature != nil && (*refAgent.Temperature < 0.0 || *refAgent.Temperature > 2.0) {
			return fmt.Errorf("Reference %d temperature %.2f is out of range (0.0–2.0)", i+1, *refAgent.Temperature)
		}
		if refAgent.MaxTokens != nil && *refAgent.MaxTokens == 0 {
			return fmt.Errorf("Reference %d max_tokens must be greater than 0", i+1)
		}
	}
	return nil
}
