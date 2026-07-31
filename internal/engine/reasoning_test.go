package engine

import "testing"

func TestSplitReasoning_TraceAndAnswer(t *testing.T) {
	raw := "⊗trace here</⊗> final answer"
	reasoning, answer := SplitReasoning(raw)
	if reasoning == nil {
		t.Fatal("expected reasoning")
	}
	if *reasoning != "trace here" {
		t.Errorf("reasoning = %q, want %q", *reasoning, "trace here")
	}
	if answer != "final answer" {
		t.Errorf("answer = %q, want %q", answer, "final answer")
	}
}

func TestSplitReasoning_NoTags(t *testing.T) {
	raw := "just an answer"
	reasoning, answer := SplitReasoning(raw)
	if reasoning != nil {
		t.Errorf("expected nil reasoning, got %q", *reasoning)
	}
	if answer != "just an answer" {
		t.Errorf("answer = %q, want %q", answer, "just an answer")
	}
}

func TestSplitReasoning_MissingClose(t *testing.T) {
	raw := "⊗unclosed reasoning"
	reasoning, answer := SplitReasoning(raw)
	if reasoning != nil {
		t.Errorf("expected nil reasoning, got %q", *reasoning)
	}
	if answer != raw {
		t.Errorf("answer = %q, want %q", answer, raw)
	}
}

func TestSplitReasoning_EmptyReasoning(t *testing.T) {
	raw := "⊗   </⊗> answer only"
	reasoning, answer := SplitReasoning(raw)
	if reasoning != nil {
		t.Errorf("expected nil reasoning for empty trace, got %q", *reasoning)
	}
	if answer != "answer only" {
		t.Errorf("answer = %q, want %q", answer, "answer only")
	}
}

func TestSplitReasoning_TrimsWhitespace(t *testing.T) {
	raw := "  ⊗ trace </⊗>   answer  "
	reasoning, answer := SplitReasoning(raw)
	if reasoning == nil || *reasoning != "trace" {
		t.Errorf("reasoning = %v, want %q", reasoning, "trace")
	}
	if answer != "answer" {
		t.Errorf("answer = %q, want %q", answer, "answer")
	}
}
