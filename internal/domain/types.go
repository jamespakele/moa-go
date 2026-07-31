// Package domain holds the shared MoA data types used by engine, synthesis,
// backend, verifier, and CLI. Keeping these types in a leaf package prevents
// import cycles between the orchestration and prompt-building packages.
package domain

// ReferenceOutput is the answer (and optional reasoning trace) from one model.
type ReferenceOutput struct {
	Label     string
	Model     string
	Output    string
	Reasoning *string
}

// FailedReference records a reference model that did not return output.
type FailedReference struct {
	Label string
	Model string
	Error string
}

// LeverageType identifies why a point needs user input.
type LeverageType string

// Leverage type constants match the Rust parser keys exactly.
const (
	Unverifiable     LeverageType = "unverifiable"
	UserSource       LeverageType = "user-source"
	Attestation      LeverageType = "attestation"
	GoldPlating      LeverageType = "gold-plating"
	ScopeCreep       LeverageType = "scope-creep"
	MissingEssential LeverageType = "missing-essential"
	NiceToHave       LeverageType = "nice-to-have"
	Fork             LeverageType = "fork"
	Deadlock         LeverageType = "deadlock"
)

// LeveragePoint is a single item surfaced during deliberation.
type LeveragePoint struct {
	LeverageType LeverageType
	Description  string
}

// ReviewSignals is the structured output of ParseReviewSignals.
type ReviewSignals struct {
	Converged      bool
	Deadlocked     bool
	LeveragePoints []LeveragePoint
}

// RoundResult captures the state after one deliberation round.
type RoundResult struct {
	Round            uint32
	ReferenceOutputs []ReferenceOutput
	FailedReferences  []FailedReference
	Review            *string
	Signals           *ReviewSignals
}

// MoaResult is the final output of the pipeline.
type MoaResult struct {
	Text           string
	OutputPath     string
	Rounds         []RoundResult
	LeveragePoints []LeveragePoint
}
