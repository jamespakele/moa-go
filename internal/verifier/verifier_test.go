package verifier

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jpakele/moa-go/internal/callctx"
	"github.com/jpakele/moa-go/internal/config"
	"github.com/jpakele/moa-go/internal/search"
)

type fakeBackend struct {
	responses map[string]string
}

func (f *fakeBackend) Complete(ctx context.Context, provider, model, prompt, thinking string, cc callctx.CallContext) (string, error) {
	key := provider + "/" + model
	if r, ok := f.responses[key]; ok {
		return r, nil
	}
	return "", errors.New("no response for " + key)
}

type fakeSearcher struct {
	results map[string]search.Result
}

func (f *fakeSearcher) Search(ctx context.Context, query string) (search.Result, error) {
	if r, ok := f.results[query]; ok {
		return r, nil
	}
	return search.Result{}, nil
}

func makeConfig(enabled bool) config.MoaConfig {
	cfg := config.DefaultConfig()
	cfg.Reference = []config.AgentSlot{
		{Provider: "ollama", Model: "ref1"},
	}
	cfg.Aggregator.Provider = "ollama"
	cfg.Aggregator.Model = "agg"
	cfg.Verifier.Enabled = enabled
	cfg.Verifier.Provider = "ollama"
	cfg.Verifier.Model = "agg"
	cfg.Search.Provider = "tavily"
	cfg.Search.MaxQueries = 5
	return cfg
}

func ptr[T any](v T) *T { return &v }

func TestVerifyDisabledReturnsUnchanged(t *testing.T) {
	cfg := makeConfig(false)
	v := New(cfg, nil, nil)
	res, err := v.Run(context.Background(), "prompt", nil, nil, "unchanged output")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.RevisedBody != "unchanged output" || res.VerificationSection != "" {
		t.Errorf("expected unchanged output and no verification, got %+v", res)
	}
}

func TestVerifyNoClaimsReturnsUnchanged(t *testing.T) {
	cfg := makeConfig(true)
	fb := &fakeBackend{responses: map[string]string{
		"ollama/agg": "[]",
	}}
	v := New(cfg, fb, nil)
	res, err := v.Run(context.Background(), "prompt", nil, nil, "unchanged output")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.RevisedBody != "unchanged output" || res.VerificationSection != "" {
		t.Errorf("expected unchanged output, got %+v", res)
	}
}

func TestVerifyExtractsClaimsAndSearches(t *testing.T) {
	cfg := makeConfig(true)
	fs := &fakeSearcher{results: map[string]search.Result{
		"claim one": {Answer: "answer one", Sources: []search.Source{{Title: "One", URL: "https://example.com/1"}}},
		"claim two": {Answer: "answer two", Sources: []search.Source{{Title: "Two", URL: "https://example.com/2"}}},
	}}
	fb := &dynamicFakeBackend{
		extract: `["claim one", "claim two"]`,
		revise:  "## Verification\n- **claim one** — supported [src: One](https://example.com/1)\n- **claim two** — contradicted [src: Two](https://example.com/2)\n\n## Revised Answer\nrevised body here",
	}
	v := New(cfg, fb, fs)

	res, err := v.Run(context.Background(), "prompt", nil, nil, "first output")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(res.VerificationSection, "claim one") || !strings.Contains(res.VerificationSection, "claim two") {
		t.Errorf("verification section missing claims: %s", res.VerificationSection)
	}
	if !strings.Contains(res.RevisedBody, "revised body here") {
		t.Errorf("revised body missing expected text: %s", res.RevisedBody)
	}
}

func TestVerifyEnabledParityWithDisabled(t *testing.T) {
	// Same fake backend returns identical first output whether verify runs or not.
	cfg := makeConfig(false)
	fb := &fakeBackend{responses: map[string]string{
		"ollama/agg": "single output",
	}}
	v := New(cfg, fb, nil)
	disabledRes, err := v.Run(context.Background(), "prompt", nil, nil, "single output")
	if err != nil {
		t.Fatalf("Run disabled: %v", err)
	}

	cfg.Verifier.Enabled = true
	fb2 := &dynamicFakeBackend{
		extract: `[]`,
		revise:  "unused",
	}
	fs := &fakeSearcher{results: map[string]search.Result{}}
	v2 := New(cfg, fb2, fs)
	enabledRes, err := v2.Run(context.Background(), "prompt", nil, nil, "single output")
	if err != nil {
		t.Fatalf("Run enabled: %v", err)
	}
	if enabledRes.RevisedBody != disabledRes.RevisedBody {
		t.Errorf("enabled/disabled output mismatch: %q vs %q", enabledRes.RevisedBody, disabledRes.RevisedBody)
	}
}

type dynamicFakeBackend struct {
	extract string
	revise  string
	calls   int
}

func (d *dynamicFakeBackend) Complete(ctx context.Context, provider, model, prompt, thinking string, cc callctx.CallContext) (string, error) {
	d.calls++
	if strings.Contains(prompt, "load-bearing factual claims") {
		return d.extract, nil
	}
	return d.revise, nil
}
