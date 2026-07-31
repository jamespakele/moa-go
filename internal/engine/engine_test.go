package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jpakele/moa-go/internal/callctx"
	"github.com/jpakele/moa-go/internal/config"
)

// fakeBackend records calls and returns canned responses.
type fakeBackend struct {
	responses map[string]string
	calls     []backendCall
}

type backendCall struct {
	provider, model, prompt string
}

func (f *fakeBackend) Complete(ctx context.Context, provider, model, prompt, thinking string, cc callctx.CallContext) (string, error) {
	f.calls = append(f.calls, backendCall{provider: provider, model: model, prompt: prompt})
	key := provider + "/" + model
	if r, ok := f.responses[key]; ok {
		return r, nil
	}
	return "", errors.New("no canned response for " + key)
}

func ptr[T any](v T) *T { return &v }

func makeSinglePassConfig() config.MoaConfig {
	cfg := config.DefaultConfig()
	cfg.Reference = []config.AgentSlot{
		{Provider: "ollama", Model: "ref1", Label: ptr("ref1")},
		{Provider: "ollama", Model: "ref2", Label: ptr("ref2")},
	}
	cfg.Synthesis.MaxRounds = 0
	return cfg
}

func TestRunSinglePass(t *testing.T) {
	cfg := makeSinglePassConfig()
	fake := &fakeBackend{responses: map[string]string{
		"ollama/ref1": "output one",
		"ollama/ref2": "output two",
		"ollama/glm-5.2:cloud": "synthesized result",
	}}
	cc := callctx.CallContext{}
	res, err := Run(context.Background(), cfg, "What is X?", cc, fake, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Text != "synthesized result" {
		t.Errorf("expected synthesized result, got %q", res.Text)
	}
	if len(res.Rounds) != 0 {
		t.Errorf("single-pass should have no rounds, got %d", len(res.Rounds))
	}
	if len(fake.calls) != 3 {
		t.Fatalf("expected 3 backend calls, got %d", len(fake.calls))
	}
	agg := fake.calls[2]
	if agg.provider != "ollama" || agg.model != "glm-5.2:cloud" {
		t.Errorf("expected aggregator call, got %s/%s", agg.provider, agg.model)
	}
	if !strings.Contains(agg.prompt, "ref1") || !strings.Contains(agg.prompt, "output one") {
		t.Errorf("aggregator prompt missing ref1 output: %s", agg.prompt)
	}
	if !strings.Contains(agg.prompt, "ref2") || !strings.Contains(agg.prompt, "output two") {
		t.Errorf("aggregator prompt missing ref2 output: %s", agg.prompt)
	}
	if res.OutputPath == "" {
		t.Error("expected output path")
	}
}

func TestRunNoReferences(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Reference = nil
	fake := &fakeBackend{}
	_, err := Run(context.Background(), cfg, "prompt", callctx.CallContext{}, fake, nil)
	if err == nil {
		t.Fatal("expected error with no references")
	}
	if !strings.Contains(err.Error(), "At least one reference model is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunAllReferencesFailed(t *testing.T) {
	cfg := makeSinglePassConfig()
	fake := &fakeBackend{responses: map[string]string{}}
	_, err := Run(context.Background(), cfg, "prompt", callctx.CallContext{}, fake, nil)
	if err == nil {
		t.Fatal("expected error when all references fail")
	}
	if !strings.Contains(err.Error(), "no outputs to synthesize") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeliberationConverges(t *testing.T) {
	cfg := makeSinglePassConfig()
	cfg.Synthesis.MaxRounds = 2
	fake := &fakeBackend{responses: map[string]string{
		"ollama/ref1": "ref1 output round1",
		"ollama/ref2": "ref2 output round1",
		"ollama/glm-5.2:cloud": "[CONVERGED]\nVerified.",
	}}
	res, err := Run(context.Background(), cfg, "What is X?", callctx.CallContext{}, fake, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(res.Rounds) != 1 {
		t.Errorf("expected 1 round, got %d", len(res.Rounds))
	}
	if !res.Rounds[0].Signals.Converged {
		t.Error("expected converged signal")
	}
}

func TestRunDeliberationRevisionRound(t *testing.T) {
	cfg := makeSinglePassConfig()
	cfg.Synthesis.MaxRounds = 2
	fake := &fakeBackend{responses: map[string]string{
		"ollama/ref1": "ref1 output round1",
		"ollama/ref2": "ref2 output round1",
		"ollama/glm-5.2:cloud": "Questions\n[CONTINUE]",
	}}
	res, err := Run(context.Background(), cfg, "What is X?", callctx.CallContext{}, fake, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(res.Rounds) != 2 {
		t.Errorf("expected 2 rounds, got %d", len(res.Rounds))
	}
}
