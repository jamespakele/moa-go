package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jpakele/moa-go/internal/backend"
	"github.com/jpakele/moa-go/internal/callctx"
	"github.com/jpakele/moa-go/internal/config"
	"github.com/jpakele/moa-go/internal/engine"
	"github.com/jpakele/moa-go/internal/search"
	_ "github.com/jpakele/moa-go/internal/search/tavily"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "moa",
	Short: "Mixture of Agents CLI utility",
	Long:  "Runs a configurable panel of reference models in parallel and synthesizes their outputs through an aggregator model.",
}

func init() {
	rootCmd.AddCommand(runCmd, addAgentCmd, removeAgentCmd, configCmd, initCmd)
}

var (
	runConfigPath     string
	runDryRun         bool
	runSkills         []string
	runFiles          []string
	runSystemPrompt   string
	runAppendPrompt   string
)

var runCmd = &cobra.Command{
	Use:   "run [prompt]",
	Short: "Execute a full MoA pipeline with the given prompt",
	Long: `Provide the prompt as a positional argument, or pass - to read from stdin:
  moa run "your prompt here"
  printf 'prompt' | moa run -`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().StringVarP(&runConfigPath, "config", "c", "", "Path to a custom moa.toml configuration file")
	runCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "Print the commands that would be executed without running them")
	runCmd.Flags().StringArrayVar(&runSkills, "skill", nil, "Skill file or directory (repeatable); first overrides [aggregator] skill")
	runCmd.Flags().StringArrayVar(&runFiles, "file", nil, "Context file (repeatable)")
	runCmd.Flags().StringVar(&runSystemPrompt, "system-prompt", "", "Override the system prompt")
	runCmd.Flags().StringVar(&runAppendPrompt, "append-system-prompt", "", "Append text to the system prompt")
}

func runRun(cmd *cobra.Command, args []string) error {
	var prompt string
	if len(args) > 0 {
		if args[0] == "-" {
			prompt = readStdin()
		} else {
			prompt = strings.Join(args, " ")
		}
	} else {
		prompt = readStdin()
	}

	cc := callctx.CallContext{
		Skills:             runSkills,
		Files:              runFiles,
		SystemPrompt:       runSystemPrompt,
		AppendSystemPrompt: runAppendPrompt,
	}

	cfgPath := runConfigPath
	if cfgPath == "" {
		cfgPath = "moa.toml"
	}

	if runDryRun {
		return runDryRunPrint(prompt, cfgPath, cc)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	applyCliSkillOverride(&cfg, runSkills)
	if len(runSkills) > 0 {
		fmt.Fprintf(os.Stderr, "[moa] Aggregator skill (from --skill): %s\n", *cfg.Aggregator.Skill)
	}

	if cfg.Backend.APIKey != nil && strings.TrimSpace(*cfg.Backend.APIKey) != "" {
		fmt.Fprintln(os.Stderr, "[moa] Using api_key from moa.toml for provider authentication.")
	}

	fmt.Fprintf(os.Stderr, "Backend: %s\n", cfg.Backend.Backend)
	fmt.Fprintf(os.Stderr, "Running MoA pipeline with %d reference models...\n", len(cfg.Reference))
	fmt.Fprintf(os.Stderr, "Aggregator: %s/%s\n", cfg.Aggregator.Provider, cfg.Aggregator.Model)
	fmt.Fprintf(os.Stderr, "Prompt: %s\n", prompt)
	if cfg.Synthesis.MaxRounds > 0 {
		fmt.Fprintf(os.Stderr, "Mode: deliberation (max %d rounds)\n", cfg.Synthesis.MaxRounds)
	}

	var searcher search.Searcher
	if cfg.Verifier.Enabled {
		searcher, err = search.NewSearcher(cfg.Search)
		if err != nil {
			return fmt.Errorf("failed to create searcher: %w", err)
		}
	}

	b := backend.New(cfg.Backend)
	result, err := engine.Run(context.Background(), cfg, prompt, cc, b, searcher)
	if err != nil {
		return fmt.Errorf("MoA pipeline failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✅ MoA pipeline complete. Output written to %s\n", result.OutputPath)
	if len(result.Rounds) > 0 {
		fmt.Fprintf(os.Stderr, "Deliberation: %d rounds completed\n", len(result.Rounds))
		if last := result.Rounds[len(result.Rounds)-1].Signals; last != nil {
			switch {
			case last.Converged:
				fmt.Fprintln(os.Stderr, "  Status: CONVERGED")
			case last.Deadlocked:
				fmt.Fprintln(os.Stderr, "  Status: DEADLOCKED")
			default:
				fmt.Fprintln(os.Stderr, "  Status: reached round limit")
			}
		}
	}
	if len(result.LeveragePoints) > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "--- Leverage Points (user input needed) ---")
		for i, lp := range result.LeveragePoints {
			fmt.Fprintf(os.Stderr, "  %d. [%s] %s\n", i+1, string(lp.LeverageType), lp.Description)
		}
		fmt.Fprintln(os.Stderr, "--- End Leverage Points ---")
		fmt.Fprintln(os.Stderr)
	}

	preview := result.Text
	if len(preview) > 500 {
		preview = preview[:500]
	}
	fmt.Fprintln(os.Stderr, "--- Preview ---")
	fmt.Fprint(os.Stderr, preview)
	if !strings.HasSuffix(preview, "\n") {
		fmt.Fprintln(os.Stderr)
	}
	if len(result.Text) > 500 {
		fmt.Fprintf(os.Stderr, "... (%d more chars)\n", len(result.Text)-500)
	}
	fmt.Fprintln(os.Stderr, "--- End Preview ---")
	return nil
}

func runDryRunPrint(prompt, cfgPath string, cc callctx.CallContext) error {
	fmt.Printf("[dry-run] moa run \"%s\"\n", prompt)
	if runConfigPath != "" {
		fmt.Printf("[dry-run] config: %s\n", cfgPath)
	}
	if len(cc.Skills) > 0 {
		fmt.Printf("[dry-run] skills: %s\n", debugSlice(cc.Skills))
	}
	if len(cc.Files) > 0 {
		fmt.Printf("[dry-run] files: %s\n", debugSlice(cc.Files))
	}
	if cc.SystemPrompt != "" {
		fmt.Println("[dry-run] system-prompt: (set)")
	}
	if cc.AppendSystemPrompt != "" {
		fmt.Println("[dry-run] append-system-prompt: (set)")
	}

	refCount := 3
	backendName := "native"
	var aggSkill *string
	if cfg, err := config.Load(cfgPath); err == nil {
		applyCliSkillOverride(&cfg, cc.Skills)
		refCount = len(cfg.Reference)
		backendName = cfg.Backend.Backend
		aggSkill = cfg.Aggregator.Skill
	}
	if aggSkill != nil {
		if len(cc.Skills) > 0 {
			fmt.Printf("[moa] Aggregator skill (from --skill): %s\n", *aggSkill)
		} else {
			fmt.Printf("[moa] Aggregator skill (from moa.toml): %s\n", *aggSkill)
		}
	}
	fmt.Printf("[dry-run] Would execute LLM completions for %d reference models via %s backend\n", refCount, backendName)
	return nil
}

func debugSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, v := range s {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%q", v))
	}
	b.WriteString("]")
	return b.String()
}

func readStdin() string {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to read from stdin")
		os.Exit(1)
	}
	return strings.TrimSpace(string(data))
}

func applyCliSkillOverride(cfg *config.MoaConfig, skills []string) {
	if len(skills) > 0 {
		s := skills[0]
		cfg.Aggregator.Skill = &s
	}
}

var addAgentCmd = &cobra.Command{
	Use:   "add-agent <provider/model>",
	Short: "Add a reference agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := "moa.toml"
		cfg, err := config.Load(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "No existing config found. Creating default config first...")
			cfg = config.DefaultConfig()
			if err := config.Save(cfg, cfgPath); err != nil {
				return fmt.Errorf("failed to write default config: %w", err)
			}
		}
		var label *string
		if addAgentLabel != "" {
			label = &addAgentLabel
		}
		if err := cfg.AddReference(args[0], label); err != nil {
			return err
		}
		if err := config.Save(cfg, cfgPath); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("Added reference agent. Total: %d\n", len(cfg.Reference))
		return nil
	},
}

var addAgentLabel string

func init() {
	addAgentCmd.Flags().StringVarP(&addAgentLabel, "label", "l", "", "Optional human-readable label")
}

var removeAgentCmd = &cobra.Command{
	Use:   "remove-agent <provider/model>",
	Short: "Remove a reference agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := "moa.toml"
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("no existing config found. Run `moa init` first")
		}
		if !cfg.RemoveReference(args[0]) {
			fmt.Fprintf(os.Stderr, "Warning: No agent matching '%s' found. No changes made.\n", args[0])
			return nil
		}
		if err := config.Save(cfg, cfgPath); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("Removed reference agent. Remaining: %d\n", len(cfg.Reference))
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := configPathFlag
		if cfgPath == "" {
			cfgPath = "moa.toml"
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "No config file found at '%s'. Run `moa init` to create a default configuration.\n", cfgPath)
			return nil
		}
		fmt.Print(configDisplay(cfg))
		return nil
	},
}

var configPathFlag string

func init() {
	configCmd.Flags().StringVarP(&configPathFlag, "config", "c", "", "Path to a custom moa.toml")
}

func configDisplay(cfg config.MoaConfig) string {
	var b strings.Builder
	b.WriteString("=== moa-go Configuration ===\n\n")
	b.WriteString("[Aggregator]\n")
	b.WriteString(fmt.Sprintf("  Provider: %s\n", cfg.Aggregator.Provider))
	b.WriteString(fmt.Sprintf("  Model:    %s\n", cfg.Aggregator.Model))
	if cfg.Aggregator.Skill != nil {
		b.WriteString(fmt.Sprintf("  Skill:    %s\n", *cfg.Aggregator.Skill))
	}
	b.WriteString("\n[Reference Models]\n")
	for i, ref := range cfg.Reference {
		label := "-"
		if ref.Label != nil {
			label = *ref.Label
		}
		b.WriteString(fmt.Sprintf("  %d. %s / %s  [label: %s]", i+1, ref.Provider, ref.Model, label))
		if ref.Skill != nil {
			b.WriteString(fmt.Sprintf("  [skill: %s]", *ref.Skill))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n[Backend]\n")
	b.WriteString(fmt.Sprintf("  Backend:        %s\n", cfg.Backend.Backend))
	if cfg.Backend.APIKey != nil {
		b.WriteString("  API key:        (set)\n")
	}
	if cfg.Backend.BaseURL != nil {
		b.WriteString(fmt.Sprintf("  Base URL:       %s\n", *cfg.Backend.BaseURL))
	}
	b.WriteString(fmt.Sprintf("  Timeout (secs): %d\n", cfg.Backend.TimeoutSecs))
	b.WriteString(fmt.Sprintf("  Ref thinking:   %s\n", cfg.Backend.ReferenceThinking))
	b.WriteString("\n[Synthesis]\n")
	b.WriteString(fmt.Sprintf("  Temperature: %g\n", cfg.Synthesis.Temperature))
	b.WriteString(fmt.Sprintf("  Max tokens: %d\n", cfg.Synthesis.MaxTokens))
	b.WriteString(fmt.Sprintf("  Output dir:  %s\n", cfg.Synthesis.OutputDir))
	mode := fmt.Sprintf("%d  (single pass)", cfg.Synthesis.MaxRounds)
	if cfg.Synthesis.MaxRounds > 0 {
		mode = fmt.Sprintf("%d  (deliberation)", cfg.Synthesis.MaxRounds)
	}
	b.WriteString(fmt.Sprintf("  Max rounds:  %s\n", mode))
	if cfg.Synthesis.BMadCompatible {
		b.WriteString("  BMad mode:   on (output as bmad spec draft)\n")
	}
	return b.String()
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a default moa.toml in the project root",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := "moa.toml"
		if _, err := os.Stat(cfgPath); err == nil && !initForce {
			return fmt.Errorf("moa.toml already exists. Use --force to overwrite")
		}
		cfg := config.DefaultConfig()
		if err := config.Save(cfg, cfgPath); err != nil {
			return fmt.Errorf("failed to write moa.toml: %w", err)
		}
		openRouterBlock := "# --- Optional: OpenRouter-backed reference model ---\n" +
			"# Uncomment the four lines below and set OPENROUTER_API_KEY in the\n" +
			"# environment (or api_key in [backend]) to add a second provider. OpenRouter\n" +
			"# always requires auth — leave this commented out for first-run / ollama-only\n" +
			"# setups. Use `moa remove-agent openrouter/anthropic/claude-3.5-sonnet`\n" +
			"# to drop the entry after uncommenting.\n" +
			"# [[reference]]\n" +
			"# provider = \"openrouter\"\n" +
			"# model = \"anthropic/claude-3.5-sonnet\"\n" +
			"# label = \"openrouter-claude\"\n"
		f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to append openrouter example: %w", err)
		}
		defer f.Close()
		if _, err := f.WriteString(openRouterBlock); err != nil {
			return fmt.Errorf("failed to append openrouter example: %w", err)
		}
		fmt.Println("Created default moa.toml")
		fmt.Println("  Aggregator: ollama/glm-5.2:cloud")
		fmt.Println("  Reference models: 3 (nemotron, qwen35, deepseek-v4-flash)")
		fmt.Println("  Optional openrouter block: commented out at the bottom of moa.toml")
		fmt.Println("  Reference thinking: low (reasoning traces captured for aggregator)")
		fmt.Println("  Max rounds: 0 (single pass — set 1+ for deliberation mode)")
		fmt.Println("")
		fmt.Println("To add openrouter: uncomment the block at the bottom of moa.toml and set OPENROUTER_API_KEY.")
		fmt.Println("To add personas, add a `skill` field to [aggregator] or [[reference]] entries.")
		fmt.Println("To enable iterative deliberation, set `max_rounds` to 1 or more.")
		fmt.Println("To output bmad-quick-dev spec drafts, set `bmad_compatible = true`.")
		fmt.Println("To enable web-search verification, set [verifier] enabled = true and TAVILY_API_KEY.")
		fmt.Println("Run `moa config` to view the full configuration.")
		return nil
	},
}

var initForce bool

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing moa.toml without warning")
}
