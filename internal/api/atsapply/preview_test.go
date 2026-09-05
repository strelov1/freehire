package atsapply

import "testing"

func TestPreviewAnswers_ResolvedFieldsAreLabelledForTheCandidate(t *testing.T) {
	fields := []MergedField{{ID: "first_name", Label: "First name", Kind: "text", Required: true}}
	answers := map[string]string{"first_name": "Ada"}

	preview := PreviewAnswers(fields, answers, false)

	if len(preview.Fields) != 1 || preview.Fields[0].Label != "First name" || preview.Fields[0].Value != "Ada" {
		t.Fatalf("preview.Fields = %+v, want First name=Ada", preview.Fields)
	}
	if len(preview.Pending) != 0 {
		t.Errorf("pending = %+v, want none", preview.Pending)
	}
}

func TestPreviewAnswers_AFieldWithNoLabelFallsBackToItsID(t *testing.T) {
	fields := []MergedField{{ID: "candidate-location", Kind: "text", Required: true}}
	answers := map[string]string{"location": "Lisbon, Portugal"}

	preview := PreviewAnswers(fields, answers, false)

	if len(preview.Fields) != 1 || preview.Fields[0].Label != "candidate-location" {
		t.Fatalf("preview.Fields = %+v, want the ID used as a label fallback", preview.Fields)
	}
}

func TestPreviewAnswers_TheResumeFieldIsOmittedNotShownBlank(t *testing.T) {
	fields := []MergedField{{ID: "resume", Label: "Resume/CV", Kind: "file", Required: true}}

	preview := PreviewAnswers(fields, nil, true)

	if len(preview.Fields) != 0 || len(preview.Pending) != 0 {
		t.Fatalf("preview = %+v, want the resume field surfaced elsewhere (the tailored CV reference), not here", preview)
	}
}

func TestPreviewAnswers_ADraftableUnmappedFieldIsMarkedWillDraft(t *testing.T) {
	fields := []MergedField{{ID: "why_us", Label: "Why do you want to work here?", Kind: "text", Required: true}}

	preview := PreviewAnswers(fields, nil, false)

	if len(preview.Pending) != 1 || preview.Pending[0].Label != "Why do you want to work here?" {
		t.Fatalf("pending = %+v, want the field named", preview.Pending)
	}
	if !preview.Pending[0].WillDraftAtSubmission {
		t.Errorf("WillDraftAtSubmission = false, want true for a required free-text field eligible for drafting")
	}
}

func TestPreviewAnswers_ANonDraftableUnmappedFieldIsNotMarkedWillDraft(t *testing.T) {
	// A sensitive label (compensation) is never drafted — draftable() excludes it.
	fields := []MergedField{{ID: "salary", Label: "Desired compensation", Kind: "text", Required: true}}

	preview := PreviewAnswers(fields, nil, false)

	if len(preview.Pending) != 1 {
		t.Fatalf("pending = %+v, want the field named", preview.Pending)
	}
	if preview.Pending[0].WillDraftAtSubmission {
		t.Errorf("WillDraftAtSubmission = true, want false for a sensitive field the drafter never touches")
	}
}

func TestPreviewAnswers_AnOptionalUnansweredFieldIsNotPending(t *testing.T) {
	fields := []MergedField{{ID: "portfolio_url", Kind: "text", Required: false}}

	preview := PreviewAnswers(fields, nil, false)

	if len(preview.Fields) != 0 || len(preview.Pending) != 0 {
		t.Fatalf("preview = %+v, want an unanswered optional field left out entirely", preview)
	}
}
