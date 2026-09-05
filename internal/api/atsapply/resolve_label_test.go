package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// Real custom questions Webflow's Greenhouse posting asked in the 2026-09-02 live smoke
// test (task 7.1) — a numeric question_NNNNN id answerKeyFor can never match, even though
// the candidate's known answer exists.
func TestResolve_MatchesAVisaSponsorshipQuestionByLabelWhenIDIsUnknown(t *testing.T) {
	fields := []MergedField{{
		ID:       "question_67131491",
		Label:    "Will you now or in the future require employment visa sponsorship to work in the country in which the job you're applying for is located?",
		Kind:     "text",
		Required: true,
	}}
	answers := map[string]string{"visa_sponsorship_needed": "No"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "No" {
		t.Fatalf("plan.Fields = %+v, want the custom question filled from visa_sponsorship_needed", plan.Fields)
	}
	if len(plan.Unmapped) != 0 {
		t.Errorf("unmapped = %+v, want none", plan.Unmapped)
	}
}

// The label match must reach the SAME "match an offered option, never guess" path a plain
// answer key already goes through — a select rendering "Yes"/"No" options resolves to the
// option's own value, not the raw answer string.
func TestResolve_LabelMatchedVisaQuestionStillMatchesAgainstOfferedOptions(t *testing.T) {
	fields := []MergedField{{
		ID:       "question_99",
		Label:    "Do you require visa sponsorship now or in the future?",
		Kind:     "select",
		Required: true,
		Options: []applyform.Option{
			{Label: "Yes", Value: "1"},
			{Label: "No", Value: "0"},
		},
	}}
	answers := map[string]string{"visa_sponsorship_needed": "No"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "0" {
		t.Fatalf("plan.Fields = %+v, want the option's platform value (0), not the raw answer text", plan.Fields)
	}
}

// A question whose label mentions neither visa nor sponsorship must not be swept in by
// accident — the rule is keyword-anchored, not a catch-all for anything work-related.
func TestResolve_LabelMatchDoesNotFireOnUnrelatedCustomQuestions(t *testing.T) {
	fields := []MergedField{{
		ID: "question_1", Label: "Where did you first hear about this role?",
		Kind: "text", Required: true,
	}}
	answers := map[string]string{"visa_sponsorship_needed": "No"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want nothing filled — this question is unrelated to visa sponsorship", plan.Fields)
	}
	if len(plan.Unmapped) != 1 {
		t.Fatalf("unmapped = %+v, want the field still parked", plan.Unmapped)
	}
}

// An id-based match, where one exists, is never shadowed by the label rules — id is the
// more specific, more trustworthy signal (Greenhouse's own standardized field name).
func TestResolve_IDMatchTakesPriorityOverLabelKeywordMatch(t *testing.T) {
	fields := []MergedField{{
		ID: "visa_sponsorship_needed", Label: "Some unrelated label",
		Kind: "text", Required: true,
	}}
	answers := map[string]string{"visa_sponsorship_needed": "No"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "No" {
		t.Fatalf("plan.Fields = %+v, want the direct id match", plan.Fields)
	}
}
