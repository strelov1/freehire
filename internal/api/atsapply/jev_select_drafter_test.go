package atsapply

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

// fakeJevSelectChooser is a jevSelectClient test double.
type fakeJevSelectChooser struct {
	answer string
	ok     bool
	err    error
	calls  int
}

func (f *fakeJevSelectChooser) Choose(context.Context, MergedField, []string, []experience.Atom) (string, bool, error) {
	f.calls++
	return f.answer, f.ok, f.err
}

func TestJevSelectDrafterChoosesThroughJevForSelectFields(t *testing.T) {
	jev := &fakeJevSelectChooser{answer: "Germany", ok: true}
	llm := &fakeDrafter{answer: "should not be used", ok: true}
	d := JevSelectDrafter{LLM: llm, Jev: jev}

	field := MergedField{ID: "q1", Label: "Country?", Kind: "select", Options: nil}
	answer, ok, err := d.Draft(context.Background(), field, GroundingContext{})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !ok || answer != "Germany" {
		t.Errorf("Draft = (%q, %v), want (\"Germany\", true)", answer, ok)
	}
	if jev.calls != 1 {
		t.Errorf("jev.calls = %d, want 1", jev.calls)
	}
	if len(llm.calls) != 0 {
		t.Errorf("llm.calls = %v, want none — the LLM must not be called when Jev answers a select field", llm.calls)
	}
}

func TestJevSelectDrafterDelegatesTextAndTextareaToLLM(t *testing.T) {
	jev := &fakeJevSelectChooser{answer: "should never be asked", ok: true}
	llm := &fakeDrafter{answer: "a free-text answer", ok: true}
	d := JevSelectDrafter{LLM: llm, Jev: jev}

	for _, kind := range []string{"text", "textarea"} {
		field := MergedField{ID: "q-" + kind, Label: "Describe yourself", Kind: kind}
		answer, ok, err := d.Draft(context.Background(), field, GroundingContext{})
		if err != nil {
			t.Fatalf("Draft(%s): %v", kind, err)
		}
		if !ok || answer != "a free-text answer" {
			t.Errorf("Draft(%s) = (%q, %v), want the LLM's answer", kind, answer, ok)
		}
	}
	if jev.calls != 0 {
		t.Errorf("jev.calls = %d, want 0 — text/textarea must never reach Jev", jev.calls)
	}
	if len(llm.calls) != 2 {
		t.Errorf("llm.calls = %v, want 2 (text, textarea)", llm.calls)
	}
}

func TestJevSelectDrafterFallsBackToLLMOnJevError(t *testing.T) {
	jev := &fakeJevSelectChooser{err: errors.New("jev: connection refused")}
	llm := &fakeDrafter{answer: "80k-120k", ok: true}
	d := JevSelectDrafter{LLM: llm, Jev: jev}

	field := MergedField{ID: "q1", Label: "Compensation?", Kind: "select"}
	answer, ok, err := d.Draft(context.Background(), field, GroundingContext{})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !ok || answer != "80k-120k" {
		t.Errorf("Draft = (%q, %v), want the LLM fallback's answer", answer, ok)
	}
	if len(llm.calls) != 1 {
		t.Errorf("llm.calls = %v, want exactly one fallback call", llm.calls)
	}
}

func TestJevSelectDrafterSkipsJevWhenUnconfigured(t *testing.T) {
	llm := &fakeDrafter{answer: "Hybrid", ok: true}
	d := JevSelectDrafter{LLM: llm, Jev: nil}

	field := MergedField{ID: "q1", Label: "Work arrangement?", Kind: "select"}
	answer, ok, err := d.Draft(context.Background(), field, GroundingContext{})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !ok || answer != "Hybrid" {
		t.Errorf("Draft = (%q, %v), want the LLM's answer", answer, ok)
	}
	if len(llm.calls) != 1 {
		t.Errorf("llm.calls = %v, want 1", llm.calls)
	}
}

func TestNewJevSelectDrafterLeavesJevNilWhenAPIKeyEmpty(t *testing.T) {
	d := NewJevSelectDrafter(&fakeDrafter{}, "")
	if d.Jev != nil {
		t.Errorf("Jev = %v, want nil when apiKey is empty", d.Jev)
	}
}

func TestNewJevSelectDrafterSetsJevWhenAPIKeyGiven(t *testing.T) {
	d := NewJevSelectDrafter(&fakeDrafter{}, "a-key")
	if d.Jev == nil {
		t.Error("Jev = nil, want non-nil when apiKey is set")
	}
}
