// Package verifier runs the optional web-search verify+revise pass on the
// aggregator's single output.
package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/jpakele/moa-go/internal/backend"
	"github.com/jpakele/moa-go/internal/callctx"
	"github.com/jpakele/moa-go/internal/config"
	"github.com/jpakele/moa-go/internal/domain"
	"github.com/jpakele/moa-go/internal/search"
)

// Verifier runs the extract-claims → search → revise pipeline.
type Verifier struct {
	cfg      config.MoaConfig
	backend  backend.Backend
	searcher search.Searcher
}

// New returns a Verifier configured from the moa config, backend, and searcher.
func New(cfg config.MoaConfig, b backend.Backend, s search.Searcher) *Verifier {
	return &Verifier{cfg: cfg, backend: b, searcher: s}
}

// Result is the outcome of a verify+revise pass.
type Result struct {
	RevisedBody         string
	VerificationSection string
}

// Run executes the verify+revise pass. When no claims are found or the
// searcher returns no evidence, it still returns a verification section.
func (v *Verifier) Run(
	ctx context.Context,
	originalPrompt string,
	refs []domain.ReferenceOutput,
	failed []domain.FailedReference,
	firstOutput string,
) (Result, error) {
	if !v.cfg.Verifier.Enabled {
		return Result{RevisedBody: firstOutput, VerificationSection: ""}, nil
	}
	if v.backend == nil {
		return Result{}, fmt.Errorf("verifier enabled but no backend provided")
	}
	claims, err := v.extractClaims(ctx, originalPrompt, refs, failed, firstOutput)
	if err != nil {
		return Result{}, fmt.Errorf("claim extraction failed: %w", err)
	}
	if len(claims) == 0 {
		return Result{RevisedBody: firstOutput, VerificationSection: ""}, nil
	}

	claimResults, err := v.searchClaims(ctx, claims)
	if err != nil {
		return Result{}, fmt.Errorf("claim search failed: %w", err)
	}

	revised, verification, err := v.revise(ctx, originalPrompt, refs, failed, firstOutput, claimResults)
	if err != nil {
		return Result{}, fmt.Errorf("revision failed: %w", err)
	}
	return Result{RevisedBody: revised, VerificationSection: verification}, nil
}

// IsEnabled reports whether the verify+revise pass is configured.
func IsEnabled(cfg config.MoaConfig) bool {
	return cfg.Verifier.Enabled
}

func (v *Verifier) verifierCallContext() callctx.CallContext {
	provider := v.cfg.Verifier.Provider
	model := v.cfg.Verifier.Model
	if provider == "" {
		provider = v.cfg.Aggregator.Provider
	}
	if model == "" {
		model = v.cfg.Aggregator.Model
	}
	return callctx.CallContext{
		Temperature: v.cfg.Verifier.Temperature,
		MaxTokens:   v.cfg.Verifier.MaxTokens,
	}
}

func (v *Verifier) callModel(ctx context.Context, provider, model, prompt string) (string, error) {
	cc := v.verifierCallContext()
	return v.backend.Complete(ctx, provider, model, prompt, "medium", cc)
}

func (v *Verifier) extractClaims(
	ctx context.Context,
	originalPrompt string,
	refs []domain.ReferenceOutput,
	failed []domain.FailedReference,
	firstOutput string,
) ([]string, error) {
	provider := v.cfg.Verifier.Provider
	model := v.cfg.Verifier.Model
	if provider == "" {
		provider = v.cfg.Aggregator.Provider
	}
	if model == "" {
		model = v.cfg.Aggregator.Model
	}

	prompt := buildExtractionPrompt(originalPrompt, refs, failed, firstOutput)
	raw, err := v.callModel(ctx, provider, model, prompt)
	if err != nil {
		return nil, err
	}
	claims, err := parseJSONStringArray(raw)
	if err != nil {
		return nil, err
	}
	max := v.cfg.Search.MaxQueries
	if max <= 0 {
		max = 5
	}
	if len(claims) > max {
		claims = claims[:max]
	}
	return claims, nil
}

func buildExtractionPrompt(
	originalPrompt string,
	refs []domain.ReferenceOutput,
	failed []domain.FailedReference,
	firstOutput string,
) string {
	var b strings.Builder
	b.WriteString("You are verifying a synthesized answer.\n\n")
	b.WriteString("## Original Prompt\n")
	b.WriteString(originalPrompt)
	b.WriteString("\n\n## Reference Model Outputs\n\n")
	for _, ref := range refs {
		b.WriteString(fmt.Sprintf("### %s\n%s\n\n", ref.Label, ref.Output))
	}
	if len(failed) > 0 {
		b.WriteString("## Failed Reference Models\n")
		for _, f := range failed {
			b.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", f.Label, f.Model, f.Error))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Synthesized Answer to Verify\n")
	b.WriteString(firstOutput)
	b.WriteString("\n\n## Instructions\n")
	b.WriteString("List the load-bearing factual claims and open questions in the answer that can be checked against external sources. Output a JSON array of strings and nothing else.")
	return b.String()
}

func parseJSONStringArray(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		parts := strings.SplitN(text, "\n", 2)
		if len(parts) == 2 {
			text = parts[1]
		}
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	var arr []string
	if err := json.Unmarshal([]byte(text), &arr); err != nil {
		return nil, fmt.Errorf("could not parse claim JSON: %w", err)
	}
	return arr, nil
}

type claimEvidence struct {
	claim  string
	result search.Result
}

func (v *Verifier) searchClaims(ctx context.Context, claims []string) ([]claimEvidence, error) {
	results := make([]claimEvidence, len(claims))
	var wg sync.WaitGroup
	for i, claim := range claims {
		wg.Add(1)
		go func(i int, claim string) {
			defer wg.Done()
			res, err := v.searcher.Search(ctx, claim)
			if err != nil {
				res = search.Result{Answer: "(search error: " + err.Error() + ")"}
			}
			results[i] = claimEvidence{claim: claim, result: res}
		}(i, claim)
	}
	wg.Wait()
	return results, nil
}

func (v *Verifier) revise(
	ctx context.Context,
	originalPrompt string,
	refs []domain.ReferenceOutput,
	failed []domain.FailedReference,
	firstOutput string,
	evidence []claimEvidence,
) (string, string, error) {
	provider := v.cfg.Verifier.Provider
	model := v.cfg.Verifier.Model
	if provider == "" {
		provider = v.cfg.Aggregator.Provider
	}
	if model == "" {
		model = v.cfg.Aggregator.Model
	}

	prompt := buildRevisionPrompt(originalPrompt, refs, failed, firstOutput, evidence)
	raw, err := v.callModel(ctx, provider, model, prompt)
	if err != nil {
		return "", "", err
	}

	verification, revised := splitSections(raw)
	if verification == "" {
		verification = buildFallbackVerification(evidence)
	}
	return revised, verification, nil
}

func buildRevisionPrompt(
	originalPrompt string,
	refs []domain.ReferenceOutput,
	failed []domain.FailedReference,
	firstOutput string,
	evidence []claimEvidence,
) string {
	var b strings.Builder
	b.WriteString("You are revising a synthesized answer after a web-search verification pass.\n\n")
	b.WriteString("## Original Prompt\n")
	b.WriteString(originalPrompt)
	b.WriteString("\n\n## Reference Model Outputs (Context)\n\n")
	for _, ref := range refs {
		b.WriteString(fmt.Sprintf("### %s\n%s\n\n", ref.Label, ref.Output))
	}
	if len(failed) > 0 {
		b.WriteString("## Failed Reference Models\n")
		for _, f := range failed {
			b.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", f.Label, f.Model, f.Error))
		}
		b.WriteString("\n")
	}
	b.WriteString("## First Synthesized Output\n")
	b.WriteString(firstOutput)
	b.WriteString("\n\n## Verification Report\n")
	for _, ev := range evidence {
		b.WriteString(fmt.Sprintf("\n### Claim: %s\n", ev.claim))
		b.WriteString(fmt.Sprintf("- Tavily answer: %s\n", ev.result.Answer))
		b.WriteString("- Top sources:\n")
		n := 3
		if len(ev.result.Sources) < n {
			n = len(ev.result.Sources)
		}
		for i := 0; i < n; i++ {
			s := ev.result.Sources[i]
			b.WriteString(fmt.Sprintf("  - [%s](%s)\n", s.Title, s.URL))
		}
		b.WriteString("- Verdict: supported / contradicted / no-evidence\n")
	}
	b.WriteString("\n## Instructions\n")
	b.WriteString("1. Produce a ## Verification section with one line per claim: `- **<claim>** — <verdict> [src: <title>](<url>)`.\n")
	b.WriteString("2. Then produce a ## Revised Answer section with the revised answer. Fix any contradicted claims, cite sources inline as [src: title], and keep everything else.\n")
	b.WriteString("3. Do not include any other sections.\n")
	return b.String()
}

// splitSections splits the model response into the verification section and the
// revised body. It expects the response to contain a ## Verification section
// and a ## Revised Answer section.
func splitSections(raw string) (string, string) {
	lower := strings.ToLower(raw)
	verIdx := strings.Index(lower, "## verification")
	revIdx := strings.Index(lower, "## revised answer")
	if verIdx == -1 || revIdx == -1 {
		return "", raw
	}
	verification := strings.TrimSpace(raw[verIdx:revIdx])
	revised := strings.TrimSpace(raw[revIdx:])
	// Drop the section headings from the body; the engine will append verification itself.
	revised = stripSectionHeading(revised)
	return verification, revised
}

func stripSectionHeading(s string) string {
	lines := strings.SplitN(s, "\n", 2)
	if len(lines) > 1 {
		return strings.TrimSpace(lines[1])
	}
	return ""
}

func buildFallbackVerification(evidence []claimEvidence) string {
	var b strings.Builder
	b.WriteString("## Verification\n\n")
	for _, ev := range evidence {
		verdict := "no-evidence"
		if ev.result.Answer != "" {
			verdict = "supported"
		}
		source := ""
		if len(ev.result.Sources) > 0 {
			s := ev.result.Sources[0]
			source = fmt.Sprintf(" [src: %s](%s)", s.Title, s.URL)
		}
		b.WriteString(fmt.Sprintf("- **%s** — %s%s\n", ev.claim, verdict, source))
	}
	return b.String()
}
