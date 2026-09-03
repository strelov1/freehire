package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

func TestResolve_FillsATextFieldFromAKnownAnswer(t *testing.T) {
	fields := []MergedField{{ID: "first_name", Kind: "text", Required: true}}
	answers := map[string]string{"first_name": "Ada"}

	plan := Resolve(fields, answers)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Ada" {
		t.Fatalf("plan.Fields = %+v, want first_name=Ada", plan.Fields)
	}
	if len(plan.Unmapped) != 0 {
		t.Errorf("unmapped = %+v, want none", plan.Unmapped)
	}
}

func TestResolve_ARequiredFieldWithNoKnownAnswerIsUnmapped(t *testing.T) {
	fields := []MergedField{{ID: "country", Label: "Country", Kind: "text", Required: true}}
	answers := map[string]string{}

	plan := Resolve(fields, answers)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want none filled", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "country" || !plan.Unmapped[0].Required {
		t.Fatalf("unmapped = %+v, want the required country field named", plan.Unmapped)
	}
}

func TestResolve_AnOptionalFieldWithNoKnownAnswerIsSkippedNotUnmapped(t *testing.T) {
	fields := []MergedField{{ID: "portfolio_url", Kind: "text", Required: false}}
	answers := map[string]string{}

	plan := Resolve(fields, answers)

	if len(plan.Fields) != 0 || len(plan.Unmapped) != 0 {
		t.Fatalf("plan = %+v, want an unanswered optional field left alone entirely", plan)
	}
}

// candidate-location is the DOM id; the answer key is "location" (the alias applies at
// resolve time too, not just at reconcile's API-matching).
func TestResolve_AppliesTheDOMToAnswerKeyAlias(t *testing.T) {
	fields := []MergedField{{ID: "candidate-location", Kind: "text", Required: true}}
	answers := map[string]string{"location": "Lisbon, Portugal"}

	plan := Resolve(fields, answers)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Lisbon, Portugal" {
		t.Fatalf("plan.Fields = %+v, want candidate-location filled from the location answer", plan.Fields)
	}
}

// A select/checkbox field's answer must match one of the platform's own offered options —
// never a value the widget does not offer, per the "never guess" rule the whole design rests
// on.
func TestResolve_MatchesAnAnswerToTheClosestOfferedOptionValue(t *testing.T) {
	fields := []MergedField{{
		ID: "authorized_countries", Kind: "select", Required: true,
		Options: []applyform.Option{{Label: "Yes", Value: "1"}, {Label: "No", Value: "0"}},
	}}
	answers := map[string]string{"authorized_countries": "Yes"}

	plan := Resolve(fields, answers)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "1" {
		t.Fatalf("plan.Fields = %+v, want the option's platform VALUE (1), not the label", plan.Fields)
	}
}

func TestResolve_AnAnswerMatchingNoOfferedOptionParksRatherThanGuessing(t *testing.T) {
	fields := []MergedField{{
		ID: "authorized_countries", Label: "Are you authorized?", Kind: "select", Required: true,
		Options: []applyform.Option{{Label: "Yes", Value: "1"}, {Label: "No", Value: "0"}},
	}}
	answers := map[string]string{"authorized_countries": "Sponsorship required"}

	plan := Resolve(fields, answers)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want nothing filled — the answer matches no offered option", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "authorized_countries" {
		t.Fatalf("unmapped = %+v, want the field named with its mismatch reason", plan.Unmapped)
	}
}

// File uploads (resume, cover letter) are not resolved from the answers map at all in this
// package yet — attaching the right stored artifact is separate plumbing this change does
// not build. A required file field always parks, explicitly, rather than silently failing at
// submit time.
func TestResolve_ARequiredFileFieldIsAlwaysUnmapped(t *testing.T) {
	fields := []MergedField{{ID: "resume", Label: "Resume/CV", Kind: "file", Required: true}}
	answers := map[string]string{"resume": "https://example.test/resume.pdf"}

	plan := Resolve(fields, answers)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want file fields never auto-filled by this package", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "resume" {
		t.Fatalf("unmapped = %+v, want the resume field named", plan.Unmapped)
	}
}

func TestResolve_FullyResolvedReportsNoUnmapped(t *testing.T) {
	fields := []MergedField{
		{ID: "first_name", Kind: "text", Required: true},
		{ID: "portfolio_url", Kind: "text", Required: false},
	}
	answers := map[string]string{"first_name": "Ada"}

	plan := Resolve(fields, answers)

	if !plan.FullyResolved() {
		t.Errorf("FullyResolved() = false, want true — the only required field is answered")
	}
}

func TestResolve_NotFullyResolvedWhenAnyUnmappedExists(t *testing.T) {
	fields := []MergedField{{ID: "country", Kind: "text", Required: true}}
	plan := Resolve(fields, map[string]string{})

	if plan.FullyResolved() {
		t.Error("FullyResolved() = true, want false — a required field has no answer")
	}
}
