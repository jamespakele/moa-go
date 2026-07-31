package synthesis

import (
	"strings"
	"testing"

	"github.com/jpakele/moa-go/internal/domain"
)

func ptr(s string) *string { return &s }

func TestBuildSynthesisPrompt_IncludesOriginalPrompt(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label:  "ref1",
		Model:  "model1",
		Output: "response from ref1",
	}}
	prompt := BuildSynthesisPrompt("What is Rust?", outputs, nil, "")
	if !strings.Contains(prompt, "What is Rust?") {
		t.Errorf("expected original prompt in synthesis prompt")
	}
	if !strings.Contains(prompt, "ref1") {
		t.Errorf("expected ref1 label")
	}
	if !strings.Contains(prompt, "response from ref1") {
		t.Errorf("expected ref1 output")
	}
}

func TestBuildSynthesisPrompt_MultipleReferences(t *testing.T) {
	outputs := []domain.ReferenceOutput{
		{Label: "nemotron", Model: "nemotron-3-super:cloud", Output: "nemotron response"},
		{Label: "qwen35", Model: "qwen3.5:397b-cloud", Output: "qwen35 response"},
	}
	prompt := BuildSynthesisPrompt("test prompt", outputs, nil, "")
	for _, want := range []string{"nemotron", "qwen35", "nemotron response", "qwen35 response"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildSynthesisPrompt_IncludesInstructions(t *testing.T) {
	outputs := []domain.ReferenceOutput{
		{Label: "ref1", Model: "model1", Output: "response 1"},
		{Label: "ref2", Model: "model2", Output: "response 2"},
	}
	prompt := BuildSynthesisPrompt("hello", outputs, nil, "")
	for _, want := range []string{
		"Identify Consensus",
		"Flag Divergence",
		"Research Disagreements",
		"Prioritize Synthesis Over Voting",
		"Attribute Sources",
		"Model Attribution",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing instruction %q", want)
		}
	}
}

func TestBuildSynthesisPrompt_SingleReferenceSkipsAgreement(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "single response",
	}}
	prompt := BuildSynthesisPrompt("hello", outputs, nil, "")
	if !strings.Contains(prompt, "Only one reference model output is available") {
		t.Errorf("expected single-reference note")
	}
	if strings.Contains(prompt, "Identify Consensus") {
		t.Errorf("single reference should not contain Identify Consensus")
	}
	if strings.Contains(prompt, "Flag Divergence") {
		t.Errorf("single reference should not contain Flag Divergence")
	}
	if !strings.Contains(prompt, "Model Attribution") {
		t.Errorf("expected Model Attribution")
	}
}

func TestBuildSynthesisPrompt_EmptyOutputs(t *testing.T) {
	prompt := BuildSynthesisPrompt("hello", nil, nil, "")
	if !strings.Contains(prompt, "hello") {
		t.Errorf("expected original prompt")
	}
	if !strings.Contains(prompt, "## Reference Model Outputs") {
		t.Errorf("expected reference outputs section")
	}
	if strings.Contains(prompt, "### ") {
		t.Errorf("did not expect any reference model heading")
	}
}

func TestBuildSynthesisPrompt_TaskModeUsesTaskHeader(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "response from ref1",
	}}
	driver := "Proceed using the system prompt and the attached context files."
	task := "Write or revise ONE story: Requirements, Acceptance Criteria, Tasks."
	prompt := BuildSynthesisPrompt(driver, outputs, nil, task)
	if !strings.Contains(prompt, "## Task\n") {
		t.Errorf("expected task header")
	}
	if !strings.Contains(prompt, task) {
		t.Errorf("expected task text")
	}
	if strings.Contains(prompt, driver) {
		t.Errorf("driver should be dropped in task mode")
	}
	if strings.Contains(prompt, "## Original Prompt") {
		t.Errorf("should not contain original prompt header in task mode")
	}
}

func TestBuildSynthesisPrompt_TaskModeProducesDeliverable(t *testing.T) {
	outputs := []domain.ReferenceOutput{
		{Label: "ref1", Model: "model1", Output: "story draft A"},
		{Label: "ref2", Model: "model2", Output: "story draft B"},
	}
	prompt := BuildSynthesisPrompt("driver", outputs, nil, "Revise the story per the PO notes.")
	if !strings.Contains(prompt, "Produce the final deliverable for the Task") {
		t.Errorf("expected deliverable instruction")
	}
	if !strings.Contains(prompt, "Do NOT write a meta-report") {
		t.Errorf("expected meta-report prohibition")
	}
	for _, unwanted := range []string{"Model Attribution", "Write your synthesis as a single, well-structured markdown document.", "Attribute Sources"} {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("task mode should not contain %q", unwanted)
		}
	}
	if !strings.Contains(prompt, "Identify Consensus") || !strings.Contains(prompt, "Flag Divergence") {
		t.Errorf("expected consensus/divergence guidance to remain")
	}
}

func TestBuildSynthesisPrompt_NoneModeKeepsLegacyReport(t *testing.T) {
	outputs := []domain.ReferenceOutput{
		{Label: "ref1", Model: "model1", Output: "response A"},
		{Label: "ref2", Model: "model2", Output: "response B"},
	}
	prompt := BuildSynthesisPrompt("What is Rust?", outputs, nil, "")
	if !strings.Contains(prompt, "## Original Prompt\nWhat is Rust?") {
		t.Errorf("expected original prompt section")
	}
	if !strings.Contains(prompt, "Model Attribution") {
		t.Errorf("expected Model Attribution")
	}
	if !strings.Contains(prompt, "Attribute Sources") {
		t.Errorf("expected Attribute Sources")
	}
	if strings.Contains(prompt, "## Task\n") {
		t.Errorf("legacy mode should not contain task header")
	}
}

func TestBuildSynthesisPrompt_WithFailedReferences(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "successful response",
	}}
	failed := []domain.FailedReference{
		{Label: "ref2", Model: "model2", Error: "timeout"},
		{Label: "ref3", Model: "model3", Error: "API error: 500"},
	}
	prompt := BuildSynthesisPrompt("test prompt", outputs, failed, "")
	for _, want := range []string{"ref1", "successful response", "## Failed Reference Models", "The following reference models failed to produce output", "ref2", "model2", "timeout", "ref3", "model3", "API error: 500"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildSynthesisPrompt_FailedReferencesNoSuccessfulOutputs(t *testing.T) {
	failed := []domain.FailedReference{{
		Label: "ref1", Model: "model1", Error: "connection refused",
	}}
	prompt := BuildSynthesisPrompt("hello", nil, failed, "")
	if !strings.Contains(prompt, "## Failed Reference Models") {
		t.Errorf("expected failed section")
	}
	if !strings.Contains(prompt, "ref1") || !strings.Contains(prompt, "model1") || !strings.Contains(prompt, "connection refused") {
		t.Errorf("expected failed reference details")
	}
	if strings.Contains(prompt, "### ") {
		t.Errorf("did not expect successful reference heading")
	}
}

func TestBuildSynthesisPrompt_NoFailedReferencesSectionWhenEmpty(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "response",
	}}
	prompt := BuildSynthesisPrompt("test", outputs, nil, "")
	if strings.Contains(prompt, "## Failed Reference Models") {
		t.Errorf("expected no failed section")
	}
}

func TestBuildSynthesisPrompt_FailedReferencesFormat(t *testing.T) {
	failed := []domain.FailedReference{{
		Label: "my-label", Model: "my-model", Error: "my-error",
	}}
	prompt := BuildSynthesisPrompt("prompt", nil, failed, "")
	expected := "- **my-label** (my-model): my-error"
	if !strings.Contains(prompt, expected) {
		t.Errorf("expected line %q, got:\n%s", expected, prompt)
	}
}

func TestBuildSynthesisPrompt_IncludesReasoningTraces(t *testing.T) {
	outputs := []domain.ReferenceOutput{
		{Label: "nemotron", Model: "nemotron-3-super:cloud", Output: "The answer is 42.", Reasoning: ptr("I considered multiple approaches and settled on 42 because it matches the Deep Thought result.")},
		{Label: "qwen35", Model: "qwen3.5:397b-cloud", Output: "42 is the answer.", Reasoning: ptr("After analyzing the question, 42 is the most logical answer.")},
	}
	prompt := BuildSynthesisPrompt("What is the answer?", outputs, nil, "")
	for _, want := range []string{"Reasoning Trace", "Deep Thought result", "most logical answer", "The answer is 42.", "42 is the answer."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildSynthesisPrompt_OmitsReasoningSectionWhenNone(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "plain answer",
	}}
	prompt := BuildSynthesisPrompt("test", outputs, nil, "")
	if strings.Contains(prompt, "Reasoning Trace") {
		t.Errorf("did not expect reasoning trace")
	}
	if !strings.Contains(prompt, "plain answer") {
		t.Errorf("expected output text")
	}
}

// --- ParseReviewSignals tests ---

func TestParseReviewSignals_Converged(t *testing.T) {
	review := "### Verified Claims\n- All claims verified\n\n### Convergence\n[CONVERGED]\n"
	signals := ParseReviewSignals(review)
	if !signals.Converged {
		t.Errorf("expected converged")
	}
	if signals.Deadlocked {
		t.Errorf("did not expect deadlocked")
	}
	if len(signals.LeveragePoints) != 0 {
		t.Errorf("expected no leverage points")
	}
}

func TestParseReviewSignals_Deadlock(t *testing.T) {
	review := "### Convergence\n[DEADLOCK]\n"
	signals := ParseReviewSignals(review)
	if !signals.Deadlocked {
		t.Errorf("expected deadlocked")
	}
	if signals.Converged {
		t.Errorf("did not expect converged")
	}
}

func TestParseReviewSignals_Continue(t *testing.T) {
	review := "### Convergence\n[CONTINUE]\n"
	signals := ParseReviewSignals(review)
	if signals.Converged || signals.Deadlocked {
		t.Errorf("expected neither converged nor deadlocked")
	}
}

func TestParseReviewSignals_LeveragePoints(t *testing.T) {
	review := "### Leverage Points\n" +
		"[LEVERAGE: unverifiable] The claim that X caused Y cannot be verified from available sources\n" +
		"[LEVERAGE: user-source] Only the user knows the internal roadmap\n" +
		"[LEVERAGE: attestation] The quote from the CEO needs user confirmation\n" +
		"[LEVERAGE: fork] Build as monolith vs microservices — strategic fork\n" +
		"[LEVERAGE: deadlock] Reference A says use Rust, Reference B says use Go\n" +
		"\n### Convergence\n[CONTINUE]\n"
	signals := ParseReviewSignals(review)
	if len(signals.LeveragePoints) != 5 {
		t.Fatalf("expected 5 leverage points, got %d", len(signals.LeveragePoints))
	}
	cases := []struct {
		idx  int
		want domain.LeverageType
	}{
		{0, domain.Unverifiable},
		{1, domain.UserSource},
		{2, domain.Attestation},
		{3, domain.Fork},
		{4, domain.Deadlock},
	}
	for _, c := range cases {
		if signals.LeveragePoints[c.idx].LeverageType != c.want {
			t.Errorf("point %d: got %q, want %q", c.idx, signals.LeveragePoints[c.idx].LeverageType, c.want)
		}
	}
	if !strings.Contains(signals.LeveragePoints[0].Description, "X caused Y") {
		t.Errorf("expected description to contain X caused Y")
	}
	if !strings.Contains(signals.LeveragePoints[3].Description, "monolith") {
		t.Errorf("expected description to contain monolith")
	}
}

func TestParseReviewSignals_ScopeLeveragePoints(t *testing.T) {
	review := "### Leverage Points\n" +
		"[LEVERAGE: gold-plating] The real-time collaboration feature is ambitious but not needed for MVP\n" +
		"[LEVERAGE: scope-creep] The proposal grew to include a plugin system — user should decide what to cut\n" +
		"[LEVERAGE: missing-essential] Error handling for the auth flow is missing\n" +
		"[LEVERAGE: nice-to-have] Dark mode would be nice but not MVP-essential\n" +
		"[LEVERAGE: fork] Monolith vs microservices — user decides\n" +
		"[LEVERAGE: deadlock] Nemotron wants PostgreSQL, Qwen wants SQLite\n" +
		"\n### Convergence\n[CONTINUE]\n"
	signals := ParseReviewSignals(review)
	if len(signals.LeveragePoints) != 6 {
		t.Fatalf("expected 6 leverage points, got %d", len(signals.LeveragePoints))
	}
	cases := []struct {
		idx  int
		want domain.LeverageType
	}{
		{0, domain.GoldPlating},
		{1, domain.ScopeCreep},
		{2, domain.MissingEssential},
		{3, domain.NiceToHave},
		{4, domain.Fork},
		{5, domain.Deadlock},
	}
	for _, c := range cases {
		if signals.LeveragePoints[c.idx].LeverageType != c.want {
			t.Errorf("point %d: got %q, want %q", c.idx, signals.LeveragePoints[c.idx].LeverageType, c.want)
		}
	}
	if !strings.Contains(signals.LeveragePoints[0].Description, "real-time collaboration") {
		t.Errorf("expected gold-plating description")
	}
	if !strings.Contains(signals.LeveragePoints[2].Description, "Error handling") {
		t.Errorf("expected missing-essential description")
	}
}

func TestParseReviewSignals_NoSignals(t *testing.T) {
	review := "### Verified Claims\n- All good\n"
	signals := ParseReviewSignals(review)
	if signals.Converged || signals.Deadlocked {
		t.Errorf("expected no signals")
	}
	if len(signals.LeveragePoints) != 0 {
		t.Errorf("expected no leverage points")
	}
}

func TestParseReviewSignals_CaseInsensitive(t *testing.T) {
	review := "[converged]\n"
	signals := ParseReviewSignals(review)
	if !signals.Converged {
		t.Errorf("expected converged (case-insensitive)")
	}
}

// --- BuildReviewPrompt tests ---

func TestBuildReviewPrompt_IncludesOriginalPrompt(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "response",
	}}
	prompt := BuildReviewPrompt("What is X?", outputs, 1, nil)
	for _, want := range []string{"What is X?", "ref1", "response"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildReviewPrompt_IncludesRoundNumber(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "response",
	}}
	prompt := BuildReviewPrompt("test", outputs, 3, nil)
	if !strings.Contains(prompt, "Round 3") {
		t.Errorf("expected Round 3")
	}
}

func TestBuildReviewPrompt_IncludesInstructions(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "response",
	}}
	prompt := BuildReviewPrompt("test", outputs, 1, nil)
	for _, want := range []string{
		"Verified", "Challenged", "[CONVERGED]", "[CONTINUE]", "[DEADLOCK]",
		"gold-plating", "scope-creep", "nice-to-have", "[DEADLOCK]",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// --- BuildRevisionPrompt tests ---

func TestBuildRevisionPrompt_IncludesPreviousOutput(t *testing.T) {
	refOut := domain.ReferenceOutput{
		Label:  "nemotron",
		Model:  "nemotron-3-super:cloud",
		Output: "The answer is 42 because...",
	}
	prompt := BuildRevisionPrompt("What is the answer?", &refOut, "Questions about sourcing", 1)
	for _, want := range []string{
		"What is the answer?", "The answer is 42 because...", "Questions about sourcing",
		"DEFEND", "CONCEDE", "REVISE",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// --- BuildFinalSynthesisPrompt tests ---

func TestBuildFinalSynthesisPrompt_IncludesDeliberationContext(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "final revised output",
	}}
	signals := domain.ReviewSignals{
		Converged: true,
		LeveragePoints: []domain.LeveragePoint{{
			LeverageType: domain.Fork,
			Description:  "monolith vs microservices",
		}},
	}
	prompt := BuildFinalSynthesisPrompt("What architecture?", outputs, nil, "All claims verified", &signals, 3)
	for _, want := range []string{
		"What architecture?", "final revised output", "All claims verified",
		"converged", "monolith vs microservices", "Leverage Points for User",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildFinalSynthesisPrompt_Deadlocked(t *testing.T) {
	outputs := []domain.ReferenceOutput{{
		Label: "ref1", Model: "model1", Output: "output",
	}}
	signals := domain.ReviewSignals{Deadlocked: true}
	prompt := BuildFinalSynthesisPrompt("test", outputs, nil, "review", &signals, 2)
	if !strings.Contains(prompt, "deadlocked") {
		t.Errorf("expected deadlocked")
	}
}
