package atsapply

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

type fakeDrafter struct {
	answer string
	ok     bool
	err    error
	calls  []string // question ids Draft was called for
}

func (f *fakeDrafter) Draft(ctx context.Context, question MergedField, grounding GroundingContext) (string, bool, error) {
	f.calls = append(f.calls, question.ID)
	return f.answer, f.ok, f.err
}

func TestResolveWithDrafting_DraftsAnOtherwiseUnmappedFreeTextField(t *testing.T) {
	fields := []MergedField{{ID: "question_1", Label: "Where did you hear about us?", Kind: "text", Required: true}}
	drafter := &fakeDrafter{answer: "Found it on the freehire job board.", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Found it on the freehire job board." {
		t.Fatalf("plan.Fields = %+v, want the drafted answer", plan.Fields)
	}
	if len(plan.Unmapped) != 0 {
		t.Errorf("unmapped = %+v, want none", plan.Unmapped)
	}
	if len(drafter.calls) != 1 || drafter.calls[0] != "question_1" {
		t.Errorf("Draft calls = %v, want exactly one for question_1", drafter.calls)
	}
}

func TestResolveWithDrafting_NeverDraftsASensitiveField(t *testing.T) {
	fields := []MergedField{{ID: "question_2", Label: "What is your desired salary?", Kind: "text", Required: true}}
	drafter := &fakeDrafter{answer: "should never be used", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(drafter.calls) != 0 {
		t.Fatalf("Draft calls = %v, want none — a sensitive field must never reach the drafter", drafter.calls)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "question_2" {
		t.Fatalf("unmapped = %+v, want the sensitive field parked", plan.Unmapped)
	}
}

func TestResolveWithDrafting_UngroundableDraftStillParks(t *testing.T) {
	fields := []MergedField{{ID: "question_3", Label: "Describe a niche hobby", Kind: "text", Required: true}}
	drafter := &fakeDrafter{ok: false} // the drafter found no basis for an answer

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want nothing filled", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "question_3" {
		t.Fatalf("unmapped = %+v, want the ungroundable field parked", plan.Unmapped)
	}
}

// The "never guess past what the widget offers" rule applies to a drafted answer exactly
// as it does to a deterministic one.
func TestResolveWithDrafting_ADraftMatchingNoOfferedOptionStillParks(t *testing.T) {
	fields := []MergedField{{
		ID: "question_4", Label: "Are you willing to relocate?", Kind: "select", Required: true,
		Options: []applyform.Option{{Label: "Yes", Value: "1"}, {Label: "No", Value: "0"}},
	}}
	drafter := &fakeDrafter{answer: "Maybe, depends on the offer", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want nothing filled — the draft matches no offered option", plan.Fields)
	}
	if len(plan.Unmapped) != 1 {
		t.Fatalf("unmapped = %+v, want the field still parked", plan.Unmapped)
	}
}

func TestResolveWithDrafting_ADraftMatchingAnOfferedOptionUsesThePlatformValue(t *testing.T) {
	fields := []MergedField{{
		ID: "question_5", Label: "Are you willing to relocate?", Kind: "select", Required: true,
		Options: []applyform.Option{{Label: "Yes", Value: "1"}, {Label: "No", Value: "0"}},
	}}
	drafter := &fakeDrafter{answer: "Yes", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "1" {
		t.Fatalf("plan.Fields = %+v, want the option's platform value (1)", plan.Fields)
	}
}

func TestResolveWithDrafting_AnOptionalFieldIsNeverDrafted(t *testing.T) {
	fields := []MergedField{{ID: "question_6", Label: "Anything else?", Kind: "text", Required: false}}
	drafter := &fakeDrafter{answer: "should never be used", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(drafter.calls) != 0 {
		t.Errorf("Draft calls = %v, want none for an optional field", drafter.calls)
	}
	if len(plan.Fields) != 0 || len(plan.Unmapped) != 0 {
		t.Errorf("plan = %+v, want the optional field left alone entirely", plan)
	}
}

func TestResolveWithDrafting_ADeterministicallyResolvedFieldIsNeverSentToTheDrafter(t *testing.T) {
	fields := []MergedField{{ID: "first_name", Kind: "text", Required: true}}
	drafter := &fakeDrafter{answer: "should never be used", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{"first_name": "Ada"}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(drafter.calls) != 0 {
		t.Errorf("Draft calls = %v, want none — the field already resolved deterministically", drafter.calls)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Ada" {
		t.Fatalf("plan.Fields = %+v, want the deterministic answer", plan.Fields)
	}
}

func TestResolveWithDrafting_ANilDrafterLeavesTheDeterministicPlanUnchanged(t *testing.T) {
	fields := []MergedField{{ID: "question_7", Label: "Anything else?", Kind: "text", Required: true}}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, nil, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Unmapped) != 1 {
		t.Fatalf("unmapped = %+v, want the field parked exactly as Resolve alone would", plan.Unmapped)
	}
}

func TestResolveWithDrafting_ADrafterErrorPropagates(t *testing.T) {
	fields := []MergedField{{ID: "question_8", Label: "Anything else?", Kind: "text", Required: true}}
	drafter := &fakeDrafter{err: errors.New("model unavailable")}

	if _, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{}); err == nil {
		t.Fatal("want the drafter's error surfaced, not swallowed into a park")
	}
}

// Found by code review: isSensitiveLabel("") is unconditionally false (an empty string
// contains no keyword), so a DOM-only field the platform's schema never labeled — exactly
// the shape reconcile.go's own test cites an EEOC/demographic field as the canonical
// example of — passed draftable's sensitivity check by having nothing to check at all. A
// field with no label can never be VERIFIED non-sensitive, so it must never be drafted.
func TestResolveWithDrafting_AFieldWithNoLabelIsNeverDrafted(t *testing.T) {
	fields := []MergedField{{ID: "42736", Label: "", Kind: "text", Required: true}}
	drafter := &fakeDrafter{answer: "should never be used", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{})
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(drafter.calls) != 0 {
		t.Fatalf("Draft calls = %v, want none — an unlabeled field cannot be verified non-sensitive", drafter.calls)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "42736" {
		t.Fatalf("unmapped = %+v, want the unlabeled field parked", plan.Unmapped)
	}
}
