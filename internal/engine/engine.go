// Package engine orchestrates the Mixture-of-Agents pipeline.
package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/jpakele/moa-go/internal/backend"
	"github.com/jpakele/moa-go/internal/callctx"
	"github.com/jpakele/moa-go/internal/config"
	"github.com/jpakele/moa-go/internal/domain"
	"github.com/jpakele/moa-go/internal/output"
	"github.com/jpakele/moa-go/internal/search"
	"github.com/jpakele/moa-go/internal/synthesis"
	"github.com/jpakele/moa-go/internal/verifier"
)

// Run executes the full MoA pipeline (single-pass or deliberation) using the
// supplied backend and call context. It returns the synthesized result and the
// path of the file it was written to.
func Run(
	ctx context.Context,
	cfg config.MoaConfig,
	prompt string,
	cc callctx.CallContext,
	b backend.Backend,
	searcher search.Searcher,
) (domain.MoaResult, error) {
	if len(cfg.Reference) == 0 {
		return domain.MoaResult{}, ErrNoReferences
	}
	if err := cfg.Validate(); err != nil {
		return domain.MoaResult{}, &ConfigValidationError{Msg: err.Error()}
	}

	// Single-pass mode (backward compatible).
	if cfg.Synthesis.MaxRounds == 0 {
		referenceOutputs, failedReferences := spawnReferences(ctx, cfg, prompt, cc, b)
		if len(referenceOutputs) == 0 {
			return domain.MoaResult{}, &AggregatorFailedError{Err: errors.New("All reference models failed — no outputs to synthesize")}
		}
		synthesisPrompt := synthesis.BuildSynthesisPrompt(prompt, referenceOutputs, failedReferences, cc.SystemPrompt)
		result, err := runAggregator(ctx, cfg, synthesisPrompt, cc, b)
		if err != nil {
			return domain.MoaResult{}, err
		}
		result, err = maybeVerifyAndRevise(ctx, cfg, prompt, referenceOutputs, failedReferences, result, b, searcher)
		if err != nil {
			return domain.MoaResult{}, err
		}
		outputPath, err := output.WriteOutput(result, cfg.Synthesis.OutputDir, prompt, cfg.Synthesis.BMadCompatible, cfg.Synthesis.BMadConfigPath)
		if err != nil {
			return domain.MoaResult{}, &OutputWriteError{Err: err}
		}
		return domain.MoaResult{Text: result, OutputPath: outputPath}, nil
	}

	// Deliberation mode (max_rounds > 0).
	maxRounds := cfg.Synthesis.MaxRounds
	var rounds []domain.RoundResult
	var allLeveragePoints []domain.LeveragePoint

	fmt.Fprintf(os.Stderr, "[moa] Deliberation round 1/%d: references proposing...\n", maxRounds)
	referenceOutputs, failedReferences := spawnReferences(ctx, cfg, prompt, cc, b)
	if len(referenceOutputs) == 0 {
		return domain.MoaResult{}, &AggregatorFailedError{Err: errors.New("All reference models failed in round 1")}
	}

	lastReview := ""
	lastSignals := domain.ReviewSignals{}

	for round := uint32(1); round <= maxRounds; round++ {
		fmt.Fprintf(os.Stderr, "[moa] Round %d: aggregator reviewing...\n", round)
		reviewPrompt := synthesis.BuildReviewPrompt(prompt, referenceOutputs, round, failedReferences)
		review, err := runAggregator(ctx, cfg, reviewPrompt, cc, b)
		if err != nil {
			return domain.MoaResult{}, err
		}
		signals := synthesis.ParseReviewSignals(review)
		fmt.Fprintf(os.Stderr, "[moa] Round %d: converged=%t, leverage=%d, deadlocked=%t\n",
			round, signals.Converged, len(signals.LeveragePoints), signals.Deadlocked)

		for _, lp := range signals.LeveragePoints {
			if !descriptionSeen(allLeveragePoints, lp.Description) {
				allLeveragePoints = append(allLeveragePoints, lp)
			}
		}

		lastReview = review
		lastSignals = signals
		rounds = append(rounds, domain.RoundResult{
			Round:            round,
			ReferenceOutputs: cloneOutputs(referenceOutputs),
			FailedReferences: cloneFailed(failedReferences),
			Review:           &review,
			Signals:          &signals,
		})

		if signals.Converged || signals.Deadlocked {
			break
		}

		if round < maxRounds {
			fmt.Fprintf(os.Stderr, "[moa] Round %d: references revising...\n", round+1)
			revisedOutputs, revisedFailures := runRevisions(ctx, cfg, prompt, cc, b, referenceOutputs, lastReview, round)
			if len(revisedOutputs) > 0 {
				referenceOutputs = revisedOutputs
				failedReferences = revisedFailures
			}
		}
	}

	fmt.Fprintln(os.Stderr, "[moa] Final synthesis...")
	finalPrompt := synthesis.BuildFinalSynthesisPrompt(prompt, referenceOutputs, failedReferences, lastReview, &lastSignals, uint32(len(rounds)))
	result, err := runAggregator(ctx, cfg, finalPrompt, cc, b)
	if err != nil {
		return domain.MoaResult{}, err
	}
	result, err = maybeVerifyAndRevise(ctx, cfg, prompt, referenceOutputs, failedReferences, result, b, searcher)
	if err != nil {
		return domain.MoaResult{}, err
	}
	outputPath, err := output.WriteOutput(result, cfg.Synthesis.OutputDir, prompt, cfg.Synthesis.BMadCompatible, cfg.Synthesis.BMadConfigPath)
	if err != nil {
		return domain.MoaResult{}, &OutputWriteError{Err: err}
	}
	return domain.MoaResult{
		Text:           result,
		OutputPath:     outputPath,
		Rounds:         rounds,
		LeveragePoints: allLeveragePoints,
	}, nil
}

func maybeVerifyAndRevise(
	ctx context.Context,
	cfg config.MoaConfig,
	prompt string,
	referenceOutputs []domain.ReferenceOutput,
	failedReferences []domain.FailedReference,
	result string,
	b backend.Backend,
	searcher search.Searcher,
) (string, error) {
	if !cfg.Verifier.Enabled || searcher == nil {
		return result, nil
	}
	fmt.Fprintln(os.Stderr, "[moa] Verifying claims...")
	v := verifier.New(cfg, b, searcher)
	vr, err := v.Run(ctx, prompt, referenceOutputs, failedReferences, result)
	if err != nil {
		return "", &AggregatorFailedError{Err: fmt.Errorf("verification pass failed: %w", err)}
	}
	fmt.Fprintf(os.Stderr, "[moa] Searched claims, revising...\n")
	if vr.VerificationSection != "" {
		return vr.RevisedBody + "\n\n" + vr.VerificationSection, nil
	}
	return vr.RevisedBody, nil
}

func runReference(
	ctx context.Context,
	slot *config.AgentSlot,
	prompt string,
	thinking string,
	b backend.Backend,
	cc callctx.CallContext,
) (domain.ReferenceOutput, error) {
	label := slot.Model
	if slot.Label != nil {
		label = *slot.Label
	}
	raw, err := b.Complete(ctx, slot.Provider, slot.Model, prompt, thinking, cc)
	if err != nil {
		return domain.ReferenceOutput{}, &ReferenceFailedError{Label: label, Model: slot.Model, Err: err}
	}
	reasoning, answer := SplitReasoning(raw)
	return domain.ReferenceOutput{Label: label, Model: slot.Model, Output: answer, Reasoning: reasoning}, nil
}

func spawnReferences(
	ctx context.Context,
	cfg config.MoaConfig,
	prompt string,
	cc callctx.CallContext,
	b backend.Backend,
) ([]domain.ReferenceOutput, []domain.FailedReference) {
	ordered := make([]*domain.ReferenceOutput, len(cfg.Reference))
	failedOrdered := make([]*domain.FailedReference, len(cfg.Reference))
	var wg sync.WaitGroup
	for i, slot := range cfg.Reference {
		wg.Add(1)
		go func(i int, slot config.AgentSlot) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					label := slot.Model
					if slot.Label != nil {
						label = *slot.Label
					}
					failedOrdered[i] = &domain.FailedReference{Label: label, Model: slot.Model, Error: fmt.Sprintf("task panicked: %v", r)}
					fmt.Fprintf(os.Stderr, "[moa] Reference task panicked: %v\n", r)
				}
			}()
			refCtx := buildReferenceContext(&slot, cc, cfg.Synthesis.MaxTokens)
			out, err := runReference(ctx, &slot, prompt, cfg.Backend.ReferenceThinking, b, refCtx)
			if err != nil {
				var rfe *ReferenceFailedError
				if errors.As(err, &rfe) {
					fmt.Fprintf(os.Stderr, "[moa] Reference failed: %s (%s) - %v\n", rfe.Label, rfe.Model, rfe.Err)
					failedOrdered[i] = &domain.FailedReference{Label: rfe.Label, Model: rfe.Model, Error: rfe.Err.Error()}
				} else {
					label := "unknown"
					model := "unknown"
					if slot.Label != nil {
						label = *slot.Label
					} else {
						label = slot.Model
						model = slot.Model
					}
					fmt.Fprintf(os.Stderr, "[moa] Reference failed: %v\n", err)
					failedOrdered[i] = &domain.FailedReference{Label: label, Model: model, Error: err.Error()}
				}
			} else {
				ordered[i] = &out
			}
		}(i, slot)
	}
	wg.Wait()

	var outputs []domain.ReferenceOutput
	for _, o := range ordered {
		if o != nil {
			outputs = append(outputs, *o)
		}
	}
	var failed []domain.FailedReference
	for _, f := range failedOrdered {
		if f != nil {
			failed = append(failed, *f)
		}
	}
	return outputs, failed
}

func runRevisions(
	ctx context.Context,
	cfg config.MoaConfig,
	prompt string,
	cc callctx.CallContext,
	b backend.Backend,
	referenceOutputs []domain.ReferenceOutput,
	lastReview string,
	round uint32,
) ([]domain.ReferenceOutput, []domain.FailedReference) {
	ordered := make([]*domain.ReferenceOutput, len(referenceOutputs))
	failedOrdered := make([]*domain.FailedReference, len(referenceOutputs))
	var wg sync.WaitGroup
	for i, refOut := range referenceOutputs {
		wg.Add(1)
		go func(i int, refOut domain.ReferenceOutput) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					failedOrdered[i] = &domain.FailedReference{Label: refOut.Label, Model: refOut.Model, Error: fmt.Sprintf("task panicked: %v", r)}
					fmt.Fprintf(os.Stderr, "[moa] Revision task panicked: %v\n", r)
				}
			}()
			revisionPrompt := synthesis.BuildRevisionPrompt(prompt, &refOut, lastReview, round)
			slot := findRevisionSlot(cfg, refOut)
			refCtx := buildReferenceContext(&slot, cc, cfg.Synthesis.MaxTokens)
			out, err := runReference(ctx, &slot, revisionPrompt, cfg.Backend.ReferenceThinking, b, refCtx)
			if err != nil {
				var rfe *ReferenceFailedError
				if errors.As(err, &rfe) {
					fmt.Fprintf(os.Stderr, "[moa] Revision failed: %s (%s) - %v\n", rfe.Label, rfe.Model, rfe.Err)
					failedOrdered[i] = &domain.FailedReference{Label: rfe.Label, Model: rfe.Model, Error: rfe.Err.Error()}
				} else {
					fmt.Fprintf(os.Stderr, "[moa] Revision failed: %v\n", err)
					failedOrdered[i] = &domain.FailedReference{Label: refOut.Label, Model: refOut.Model, Error: err.Error()}
				}
			} else {
				ordered[i] = &out
			}
		}(i, refOut)
	}
	wg.Wait()

	var outputs []domain.ReferenceOutput
	for _, o := range ordered {
		if o != nil {
			outputs = append(outputs, *o)
		}
	}
	var failed []domain.FailedReference
	for _, f := range failedOrdered {
		if f != nil {
			failed = append(failed, *f)
		}
	}
	return outputs, failed
}

func findRevisionSlot(cfg config.MoaConfig, refOut domain.ReferenceOutput) config.AgentSlot {
	for _, s := range cfg.Reference {
		label := s.Model
		if s.Label != nil {
			label = *s.Label
		}
		if label == refOut.Label {
			return s
		}
	}
	fmt.Fprintf(os.Stderr, "[moa] Warning: could not find slot for label '%s', using first reference slot\n", refOut.Label)
	fallback := cfg.Reference[0]
	fallback.Label = &refOut.Label
	fallback.Model = refOut.Model
	fallback.Skill = nil
	fallback.Temperature = nil
	fallback.MaxTokens = nil
	return fallback
}

func buildReferenceContext(slot *config.AgentSlot, cc callctx.CallContext, synthesisMaxTokens uint32) callctx.CallContext {
	refCtx := cc
	if slot.Skill != nil {
		refCtx.Skills = append([]string{*slot.Skill}, refCtx.Skills...)
	}
	if slot.Temperature != nil {
		refCtx.Temperature = slot.Temperature
	}
	if slot.MaxTokens != nil {
		refCtx.MaxTokens = slot.MaxTokens
	} else {
		refCtx.MaxTokens = &synthesisMaxTokens
	}
	return refCtx
}

func buildAggregatorContext(cfg config.MoaConfig, cc callctx.CallContext) callctx.CallContext {
	aggCtx := callctx.CallContext{
		SystemPrompt:       cc.SystemPrompt,
		AppendSystemPrompt: cc.AppendSystemPrompt,
	}
	if cfg.Aggregator.Skill != nil {
		aggCtx.Skills = append(aggCtx.Skills, *cfg.Aggregator.Skill)
	}
	if cfg.Aggregator.Temperature != nil {
		aggCtx.Temperature = cfg.Aggregator.Temperature
	}
	if cfg.Aggregator.MaxTokens != nil {
		aggCtx.MaxTokens = cfg.Aggregator.MaxTokens
	} else {
		maxTokens := cfg.Synthesis.MaxTokens
		aggCtx.MaxTokens = &maxTokens
	}
	return aggCtx
}

func runAggregator(
	ctx context.Context,
	cfg config.MoaConfig,
	synthesisPrompt string,
	cc callctx.CallContext,
	b backend.Backend,
) (string, error) {
	aggCtx := buildAggregatorContext(cfg, cc)
	raw, err := b.Complete(ctx, cfg.Aggregator.Provider, cfg.Aggregator.Model, synthesisPrompt, "medium", aggCtx)
	if err != nil {
		return "", &AggregatorFailedError{Err: err}
	}
	return raw, nil
}

func descriptionSeen(points []domain.LeveragePoint, desc string) bool {
	for _, p := range points {
		if p.Description == desc {
			return true
		}
	}
	return false
}

func cloneOutputs(in []domain.ReferenceOutput) []domain.ReferenceOutput {
	out := make([]domain.ReferenceOutput, len(in))
	copy(out, in)
	return out
}

func cloneFailed(in []domain.FailedReference) []domain.FailedReference {
	out := make([]domain.FailedReference, len(in))
	copy(out, in)
	return out
}
