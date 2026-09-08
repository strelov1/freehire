package atsapply

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/coverletter"
)

// fakeLetterReader is the LetterReader test double — mirrors fakeDrafter's shape.
type fakeLetterReader struct {
	stored *coverletter.Stored
	err    error
	calls  int
}

func (f *fakeLetterReader) Get(ctx context.Context, userID, jobID int64) (*coverletter.Stored, error) {
	f.calls++
	return f.stored, f.err
}

func TestCoverLetterAnswer_ReturnsTheStoredLetterBody(t *testing.T) {
	reader := &fakeLetterReader{stored: &coverletter.Stored{Letter: coverletter.Letter{Body: "Dear hiring team, ..."}}}

	answer, ok := coverLetterAnswer(context.Background(), reader, 1, 2)
	if !ok || answer != "Dear hiring team, ..." {
		t.Fatalf("coverLetterAnswer = (%q, %v), want the stored letter body", answer, ok)
	}
}

func TestCoverLetterAnswer_NoStoredLetterIsNotAnAnswer(t *testing.T) {
	reader := &fakeLetterReader{stored: nil}

	if _, ok := coverLetterAnswer(context.Background(), reader, 1, 2); ok {
		t.Error("want ok=false when no letter has been drafted yet")
	}
}

func TestCoverLetterAnswer_AReadErrorDegradesRatherThanPanics(t *testing.T) {
	reader := &fakeLetterReader{err: errors.New("db unavailable")}

	answer, ok := coverLetterAnswer(context.Background(), reader, 1, 2)
	if ok || answer != "" {
		t.Fatalf("coverLetterAnswer = (%q, %v), want a read error treated as no answer", answer, ok)
	}
}

func TestResolveWithDrafting_ACoverLetterFieldWithAnExistingLetterUsesItAndNeverCallsTheDrafter(t *testing.T) {
	fields := []MergedField{{ID: "cover_letter_text", Label: "Cover Letter", Kind: "textarea", Required: true}}
	drafter := &fakeDrafter{answer: "should never be used", ok: true}
	letters := &fakeLetterReader{stored: &coverletter.Stored{Letter: coverletter.Letter{Body: "My tailored letter for this job."}}}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{}, false, letters, 1, 2)
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "My tailored letter for this job." {
		t.Fatalf("plan.Fields = %+v, want the stored letter used verbatim", plan.Fields)
	}
	if len(drafter.calls) != 0 {
		t.Errorf("Draft calls = %v, want none — the existing letter must be preferred over the generic drafter", drafter.calls)
	}
	if letters.calls != 1 {
		t.Errorf("letters.Get calls = %d, want exactly 1", letters.calls)
	}
}

func TestResolveWithDrafting_ACoverLetterFieldWithNoExistingLetterFallsBackToTheGenericDrafter(t *testing.T) {
	fields := []MergedField{{ID: "cover_letter_text", Label: "Cover Letter", Kind: "textarea", Required: true}}
	drafter := &fakeDrafter{answer: "a short grounded answer", ok: true}
	letters := &fakeLetterReader{stored: nil}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{}, false, letters, 1, 2)
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "a short grounded answer" {
		t.Fatalf("plan.Fields = %+v, want the generic drafter's answer, exactly as before this change", plan.Fields)
	}
	if len(drafter.calls) != 1 {
		t.Errorf("Draft calls = %v, want exactly one fallback call", drafter.calls)
	}
}

func TestResolveWithDrafting_AUnrelatedFreeTextFieldIgnoresAnExistingLetter(t *testing.T) {
	fields := []MergedField{{ID: "question_1", Label: "Why do you want to work here?", Kind: "text", Required: true}}
	drafter := &fakeDrafter{answer: "Found it on the freehire job board.", ok: true}
	letters := &fakeLetterReader{stored: &coverletter.Stored{Letter: coverletter.Letter{Body: "My tailored letter for this job."}}}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{}, false, letters, 1, 2)
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Found it on the freehire job board." {
		t.Fatalf("plan.Fields = %+v, want the drafter's answer — an unrelated field must ignore the letter store", plan.Fields)
	}
	if letters.calls != 0 {
		t.Errorf("letters.Get calls = %d, want none — the letter store is only consulted for a cover-letter field", letters.calls)
	}
}

func TestResolveWithDrafting_ANilLetterReaderLeavesCoverLetterFieldsToTheGenericDrafter(t *testing.T) {
	fields := []MergedField{{ID: "cover_letter_text", Label: "Cover Letter", Kind: "textarea", Required: true}}
	drafter := &fakeDrafter{answer: "a short grounded answer", ok: true}

	plan, err := ResolveWithDrafting(context.Background(), fields, map[string]string{}, drafter, GroundingContext{}, false, nil, 1, 2)
	if err != nil {
		t.Fatalf("ResolveWithDrafting: %v", err)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "a short grounded answer" {
		t.Fatalf("plan.Fields = %+v, want the generic drafter's answer when no letter reader is configured", plan.Fields)
	}
}
