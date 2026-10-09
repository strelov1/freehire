package autofillagent

import (
	"context"
	"errors"
	"testing"
)

// fakeJevChooser is a jevClient test double: it returns a fixed answer or error, without
// ever reaching the network.
type fakeJevChooser struct {
	choice string
	err    error
	calls  int
}

func (f *fakeJevChooser) Choose(context.Context, Field, []string, Profile) (string, error) {
	f.calls++
	return f.choice, f.err
}

// fakeLLMChoosePlanner is a Planner test double recording whether Choose/Plan were called,
// so tests can assert on which backend actually answered.
type fakeLLMChoosePlanner struct {
	choice     string
	chooseErr  error
	chooseCall int
	planCall   int
}

func (f *fakeLLMChoosePlanner) Plan(context.Context, []Field, Profile) ([]Fill, error) {
	f.planCall++
	return nil, nil
}

func (f *fakeLLMChoosePlanner) Choose(context.Context, Field, []string, Profile) (string, error) {
	f.chooseCall++
	return f.choice, f.chooseErr
}

func TestJevPlannerChoosesThroughJevWhenConfigured(t *testing.T) {
	jev := &fakeJevChooser{choice: "Germany"}
	llm := &fakeLLMChoosePlanner{choice: "should not be used"}
	p := JevPlanner{LLM: llm, Jev: jev}

	got, err := p.Choose(context.Background(), Field{Label: "Country?"}, []string{"Germany", "France"}, Profile{})
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "Germany" {
		t.Errorf("Choose = %q, want %q", got, "Germany")
	}
	if jev.calls != 1 {
		t.Errorf("jev.calls = %d, want 1", jev.calls)
	}
	if llm.chooseCall != 0 {
		t.Errorf("llm.chooseCall = %d, want 0 — the LLM must not be called when Jev answers", llm.chooseCall)
	}
}

func TestJevPlannerFallsBackToLLMOnJevError(t *testing.T) {
	jev := &fakeJevChooser{err: errors.New("jev: connection refused")}
	llm := &fakeLLMChoosePlanner{choice: "2 weeks"}
	p := JevPlanner{LLM: llm, Jev: jev}

	got, err := p.Choose(context.Background(), Field{Label: "Notice period?"}, []string{"2 weeks", "1 month"}, Profile{})
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "2 weeks" {
		t.Errorf("Choose = %q, want %q (the LLM fallback's answer)", got, "2 weeks")
	}
	if llm.chooseCall != 1 {
		t.Errorf("llm.chooseCall = %d, want 1 — the fallback must run on a Jev error", llm.chooseCall)
	}
}

func TestJevPlannerSkipsJevWhenUnconfigured(t *testing.T) {
	llm := &fakeLLMChoosePlanner{choice: "Yes"}
	p := JevPlanner{LLM: llm, Jev: nil}

	got, err := p.Choose(context.Background(), Field{Label: "Relocate?"}, []string{"Yes", "No"}, Profile{})
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "Yes" {
		t.Errorf("Choose = %q, want %q", got, "Yes")
	}
	if llm.chooseCall != 1 {
		t.Errorf("llm.chooseCall = %d, want 1", llm.chooseCall)
	}
}

func TestNewJevPlannerLeavesJevNilWhenAPIKeyEmpty(t *testing.T) {
	p := NewJevPlanner(&fakeLLMChoosePlanner{}, "")
	if p.Jev != nil {
		t.Errorf("Jev = %v, want nil when apiKey is empty", p.Jev)
	}
}

func TestNewJevPlannerSetsJevWhenAPIKeyGiven(t *testing.T) {
	p := NewJevPlanner(&fakeLLMChoosePlanner{}, "a-key")
	if p.Jev == nil {
		t.Error("Jev = nil, want non-nil when apiKey is set")
	}
}

func TestJevPlannerPlanAlwaysDelegatesToLLM(t *testing.T) {
	jev := &fakeJevChooser{choice: "should never be asked"}
	llm := &fakeLLMChoosePlanner{}
	p := JevPlanner{LLM: llm, Jev: jev}

	if _, err := p.Plan(context.Background(), []Field{{Label: "Email"}}, Profile{}); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if llm.planCall != 1 {
		t.Errorf("llm.planCall = %d, want 1", llm.planCall)
	}
	if jev.calls != 0 {
		t.Errorf("jev.calls = %d, want 0 — Plan must never reach Jev", jev.calls)
	}
}
