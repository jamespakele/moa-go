// Package synthesis builds prompts and parses aggregator review signals.
package synthesis

import (
	"strings"

	"github.com/jpakele/moa-go/internal/domain"
)

// BuildSynthesisPrompt constructs the aggregator prompt for single-pass mode.
// An empty task means "no task" (Rust None); the original prompt is used as the
// deliverable in that case.
func BuildSynthesisPrompt(
	originalPrompt string,
	referenceOutputs []domain.ReferenceOutput,
	failedReferences []domain.FailedReference,
	task string,
) string {
	var b strings.Builder

	b.WriteString("You are synthesizing outputs from multiple AI models into a single, coherent response.\n\n")

	if task != "" {
		b.WriteString("## Task\n")
		b.WriteString(task)
	} else {
		b.WriteString("## Original Prompt\n")
		b.WriteString(originalPrompt)
	}
	b.WriteString("\n\n## Reference Model Outputs\n\n")

	for _, refOut := range referenceOutputs {
		b.WriteString("### ")
		b.WriteString(refOut.Label)
		b.WriteString("\n")
		if refOut.Reasoning != nil {
			b.WriteString("\n<!-- Reasoning Trace from ")
			b.WriteString(refOut.Label)
			b.WriteString(" -->\n")
			b.WriteString(*refOut.Reasoning)
			b.WriteString("\n\n")
		}
		b.WriteString(refOut.Output)
		b.WriteString("\n\n")
	}

	if len(failedReferences) > 0 {
		b.WriteString("## Failed Reference Models\n\n")
		b.WriteString("The following reference models failed to produce output. Consider this when evaluating the synthesis.\n\n")
		for _, failed := range failedReferences {
			b.WriteString("- **")
			b.WriteString(failed.Label)
			b.WriteString("** (")
			b.WriteString(failed.Model)
			b.WriteString("): ")
			b.WriteString(failed.Error)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Synthesis Instructions\n\n")

	if len(referenceOutputs) <= 1 {
		b.WriteString("Note: Only one reference model output is available, so agreement/divergence\n")
		b.WriteString("analysis is not applicable. Synthesize the single available response into\n")
		b.WriteString("a coherent answer.\n\n")
	} else {
		b.WriteString("1. **Identify Consensus**: Where reference models agree, treat this as a\n")
		b.WriteString("   high-confidence signal. Explicitly note areas of agreement.\n\n")
		b.WriteString("2. **Flag Divergence**: Where models disagree, clearly mark the\n")
		b.WriteString("   contradiction. Do not hide or smooth over disagreements.\n\n")
		b.WriteString("3. **Research Disagreements**: For each point of divergence, reason through:\n")
		b.WriteString("   - Why might the models disagree?\n")
		b.WriteString("   - Which position is better supported by evidence or logic?\n")
		b.WriteString("   - Is there a synthesis that reconciles the views?\n\n")
		b.WriteString("4. **Prioritize Synthesis Over Voting**: Do not simply count which model\n")
		b.WriteString("   \"won\" on each point. Build a coherent narrative that weighs arguments\n")
		b.WriteString("   by quality, not by quantity of models endorsing them.\n\n")
		if task == "" {
			b.WriteString("5. **Attribute Sources**: Tag claims with which reference model(s)\n")
			b.WriteString("   contributed them. Use inline citations like [nemotron], [qwen35],\n")
			b.WriteString("   [deepseek], or [consensus] for agreement.\n\n")
		}
	}

	b.WriteString("## Output Format\n\n")
	if task != "" {
		b.WriteString("Produce the final deliverable for the Task above, using the Reference Model\n")
		b.WriteString("Outputs as input. Synthesize the reference outputs into a single coherent\n")
		b.WriteString("result that fulfills the Task — in the exact format the Task specifies.\n")
		b.WriteString("Do NOT write a meta-report, a model-attribution summary, or a narrative about\n")
		b.WriteString("what the models said — output the actual result the Task asks for. You may\n")
		b.WriteString("reason about consensus and divergence, but the final output must be the\n")
		b.WriteString("deliverable itself, not commentary on the models.\n")
	} else {
		b.WriteString("Write your synthesis as a single, well-structured markdown document.\n")
		b.WriteString("End with a brief \"Model Attribution\" section summarizing which models\n")
		b.WriteString("contributed which insights.\n")
	}

	return b.String()
}

// BuildReviewPrompt constructs the adversarial review prompt for a deliberation round.
func BuildReviewPrompt(
	originalPrompt string,
	referenceOutputs []domain.ReferenceOutput,
	round uint32,
	failedReferences []domain.FailedReference,
) string {
	var b strings.Builder

	b.WriteString("You are the adversarial reviewer in a multi-round deliberation.\n")
	b.WriteString("Your role: sort what holds from what doesn't. Be rigorous.\n\n")

	b.WriteString("## Original Prompt\n")
	b.WriteString(originalPrompt)
	b.WriteString("\n\n")

	b.WriteString("## Reference Model Outputs (Round ")
	b.WriteString(itoa(round))
	b.WriteString(")\n\n")
	for _, refOut := range referenceOutputs {
		b.WriteString("### ")
		b.WriteString(refOut.Label)
		b.WriteString("\n")
		if refOut.Reasoning != nil {
			b.WriteString("\n<!-- Reasoning Trace from ")
			b.WriteString(refOut.Label)
			b.WriteString(" -->\n")
			b.WriteString(*refOut.Reasoning)
			b.WriteString("\n\n")
		}
		b.WriteString(refOut.Output)
		b.WriteString("\n\n")
	}

	if len(failedReferences) > 0 {
		b.WriteString("## Failed Reference Models\n\n")
		for _, failed := range failedReferences {
			b.WriteString("- **")
			b.WriteString(failed.Label)
			b.WriteString("** (")
			b.WriteString(failed.Model)
			b.WriteString("): ")
			b.WriteString(failed.Error)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Review Instructions\n\n")
	b.WriteString("Review with rigor. Your persona (loaded via your skill) shapes what you\n")
	b.WriteString("focus on — follow it.\n\n")
	b.WriteString("1. **Sort what holds from what doesn't** — identify what's solid vs what\n")
	b.WriteString("   needs challenge.\n\n")
	b.WriteString("2. **Question the references** — for anything that needs challenge, write\n")
	b.WriteString("   a specific question or correction for the relevant model.\n\n")
	b.WriteString("3. **Flag what's contested** — where references disagree, name the conflict.\n\n")
	b.WriteString("4. **Surface what only the user can decide** — if proceeding would mean\n")
	b.WriteString("   guessing or committing the user, flag it as a leverage point.\n\n")

	b.WriteString("## Convergence Signals\n\n")
	b.WriteString("Output one of these at the end of your review:\n")
	b.WriteString("- `[CONVERGED]` — nothing left to challenge, references are solid\n")
	b.WriteString("- `[CONTINUE]` — corrections needed, references should respond\n")
	b.WriteString("- `[DEADLOCK]` — unresolvable disagreement between references\n\n")
	b.WriteString("If user input is required, output leverage points (one per line).\n")
	b.WriteString("Use whichever types apply:\n\n")
	b.WriteString("Research / sourcing:\n")
	b.WriteString("- `[LEVERAGE: unverifiable] <desc>` — load-bearing claim that can't be verified\n")
	b.WriteString("- `[LEVERAGE: user-source] <desc>` — source only the user knows\n")
	b.WriteString("- `[LEVERAGE: attestation] <desc>` — quote/figure needing user attestation\n\n")
	b.WriteString("Scope / MVP:\n")
	b.WriteString("- `[LEVERAGE: gold-plating] <desc>` — feature beyond MVP, user decides\n")
	b.WriteString("- `[LEVERAGE: scope-creep] <desc>` — grew beyond original scope\n")
	b.WriteString("- `[LEVERAGE: missing-essential] <desc>` — something essential was missed\n")
	b.WriteString("- `[LEVERAGE: nice-to-have] <desc>` — valuable but not MVP-essential\n\n")
	b.WriteString("General:\n")
	b.WriteString("- `[LEVERAGE: fork] <desc>` — strategic fork requiring user decision\n")
	b.WriteString("- `[LEVERAGE: deadlock] <desc>` — genuine deadlock on a specific point\n\n")

	b.WriteString("## Output Format\n\n")
	b.WriteString("### Verified\n(What holds up)\n- ...\n\n")
	b.WriteString("### Challenged\n(What needs correction or defense)\n- ...\n\n")
	b.WriteString("### Questions for References\n1. ...\n\n")
	b.WriteString("### Leverage Points\n(List any, one per line)\n\n")
	b.WriteString("### Convergence\n[CONVERGED] or [CONTINUE] or [DEADLOCK]\n")

	return b.String()
}

// BuildRevisionPrompt constructs the prompt sent to one reference model to revise
// its previous output in response to the aggregator's review.
func BuildRevisionPrompt(
	originalPrompt string,
	refOutput *domain.ReferenceOutput,
	review string,
	round uint32,
) string {
	var b strings.Builder

	b.WriteString("You previously responded to a prompt. An adversarial reviewer has raised\n")
	b.WriteString("questions and corrections. Your job: defend what you can source, concede\n")
	b.WriteString("what you can't, and revise your response.\n\n")

	b.WriteString("## Original Prompt\n")
	b.WriteString(originalPrompt)
	b.WriteString("\n\n")

	b.WriteString("## Your Previous Response (Round ")
	b.WriteString(itoa(round))
	b.WriteString(")\n")
	b.WriteString(refOutput.Output)
	b.WriteString("\n\n")

	if refOutput.Reasoning != nil {
		b.WriteString("## Your Previous Reasoning\n")
		b.WriteString(*refOutput.Reasoning)
		b.WriteString("\n\n")
	}

	b.WriteString("## Reviewer's Notes\n")
	b.WriteString(review)
	b.WriteString("\n\n")

	b.WriteString("## Your Instructions\n")
	b.WriteString("1. For each question the reviewer raised about YOUR output:\n")
	b.WriteString("   - DEFEND with specific sourcing if you can\n")
	b.WriteString("   - CONCEDE if you cannot source it\n")
	b.WriteString("   - REVISE your response to address the correction\n")
	b.WriteString("2. Do not change claims that the reviewer verified\n")
	b.WriteString("3. If the reviewer flagged a contested source, either provide\n")
	b.WriteString("   the correct source or concede the point\n")
	b.WriteString("4. Output your full revised response\n")

	return b.String()
}

// BuildFinalSynthesisPrompt constructs the prompt for the final aggregator pass
// after deliberation rounds have completed.
func BuildFinalSynthesisPrompt(
	originalPrompt string,
	referenceOutputs []domain.ReferenceOutput,
	failedReferences []domain.FailedReference,
	lastReview string,
	signals *domain.ReviewSignals,
	totalRounds uint32,
) string {
	var b strings.Builder

	b.WriteString("You are the final synthesizer. Multiple rounds of proposal, adversarial\n")
	b.WriteString("review, and revision have completed (")
	b.WriteString(itoa(totalRounds))
	b.WriteString(" rounds).\n")
	b.WriteString("Produce the final output.\n\n")

	b.WriteString("## Original Prompt\n")
	b.WriteString(originalPrompt)
	b.WriteString("\n\n")

	b.WriteString("## Final Reference Model Outputs (Last Round)\n\n")
	for _, refOut := range referenceOutputs {
		b.WriteString("### ")
		b.WriteString(refOut.Label)
		b.WriteString("\n")
		if refOut.Reasoning != nil {
			b.WriteString("\n<!-- Reasoning Trace from ")
			b.WriteString(refOut.Label)
			b.WriteString(" -->\n")
			b.WriteString(*refOut.Reasoning)
			b.WriteString("\n\n")
		}
		b.WriteString(refOut.Output)
		b.WriteString("\n\n")
	}

	if len(failedReferences) > 0 {
		b.WriteString("## Failed Reference Models\n\n")
		for _, failed := range failedReferences {
			b.WriteString("- **")
			b.WriteString(failed.Label)
			b.WriteString("** (")
			b.WriteString(failed.Model)
			b.WriteString("): ")
			b.WriteString(failed.Error)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Final Review (Adversarial)\n")
	b.WriteString(lastReview)
	b.WriteString("\n\n")

	if len(signals.LeveragePoints) > 0 {
		b.WriteString("## Leverage Points Requiring User Input\n\n")
		for i, lp := range signals.LeveragePoints {
			b.WriteString(itoa(uint32(i + 1)))
			b.WriteString(". [")
			b.WriteString(string(lp.LeverageType))
			b.WriteString("] ")
			b.WriteString(lp.Description)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Synthesis Instructions\n\n")
	b.WriteString("1. Synthesize the VERIFIED claims into a coherent response\n")
	b.WriteString("2. Exclude QUARANTINED claims that were never defended\n")
	b.WriteString("3. Attribute claims to their sources\n")
	b.WriteString("4. If leverage points exist, list them at the end under\n")
	b.WriteString("   \"## Leverage Points for User\"\n")
	if signals.Converged {
		b.WriteString("5. The deliberation converged — all claims were verified\n")
	} else if signals.Deadlocked {
		b.WriteString("5. The deliberation deadlocked — surface the unresolved\n")
		b.WriteString("   disagreement clearly in your output\n")
	} else {
		b.WriteString("5. The deliberation reached the round limit — synthesize\n")
		b.WriteString("   what was resolved and flag what remains open\n")
	}

	b.WriteString("\n## Output Format\n\n")
	b.WriteString("Write your synthesis as a single, well-structured markdown document.\n")
	b.WriteString("If leverage points exist, end with a \"## Leverage Points for User\"\n")
	b.WriteString("section listing each one with context.\n")

	return b.String()
}

// ParseReviewSignals scans an aggregator review for convergence markers and
// leverage points. Marker matching is case-insensitive and anchored to the
// start of a line, matching the Rust implementation exactly.
func ParseReviewSignals(review string) domain.ReviewSignals {
	var signals domain.ReviewSignals

	for _, line := range strings.Split(review, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(lower, "[converged]") {
			signals.Converged = true
		}
		if strings.HasPrefix(lower, "[deadlock]") && !strings.Contains(lower, "leverage") {
			signals.Deadlocked = true
		}

		if rest, ok := strings.CutPrefix(lower, "[leverage: "); ok {
			if end := strings.Index(rest, "]"); end >= 0 {
				typeStr := rest[:end]
				descStart := strings.Index(trimmed, "]")
				if descStart < 0 {
					descStart = 0
				}
				description := strings.TrimSpace(trimmed[descStart+1:])

				var leverageType domain.LeverageType
				switch typeStr {
				case "unverifiable":
					leverageType = domain.Unverifiable
				case "user-source", "user_source", "user source":
					leverageType = domain.UserSource
				case "attestation":
					leverageType = domain.Attestation
				case "gold-plating", "gold_plating", "goldplating":
					leverageType = domain.GoldPlating
				case "scope-creep", "scope_creep", "scopecreep":
					leverageType = domain.ScopeCreep
				case "missing-essential", "missing_essential":
					leverageType = domain.MissingEssential
				case "nice-to-have", "nice_to_have", "nicetohave":
					leverageType = domain.NiceToHave
				case "fork":
					leverageType = domain.Fork
				case "deadlock":
					leverageType = domain.Deadlock
				default:
					continue
				}

				signals.LeveragePoints = append(signals.LeveragePoints, domain.LeveragePoint{
					LeverageType: leverageType,
					Description:  description,
				})
			}
		}
	}

	return signals
}

// itoa formats a uint32 as a decimal string. It avoids pulling in strconv for a
// single small helper and keeps the prompt builder dependency-free.
func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
