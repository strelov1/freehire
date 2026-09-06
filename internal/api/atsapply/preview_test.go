package atsapply

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

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

// fakeFetcher records whether it was called and returns a canned form — used to prove
// PreviewClient prefers a StoredFormReader over a live fetch.
type fakeFetcher struct {
	called bool
	form   applyform.Form
	err    error
}

func (f *fakeFetcher) Fetch(context.Context, applyform.Claimed) (applyform.Form, error) {
	f.called = true
	return f.form, f.err
}

// fakeFormReader is a StoredFormReader test double.
type fakeFormReader struct {
	form  applyform.Form
	found bool
	err   error
}

func (r *fakeFormReader) GetStoredForm(context.Context, int64) (applyform.Form, bool, error) {
	return r.form, r.found, r.err
}

func TestPreviewClient_ALeverAttemptParksWithoutTouchingAFetcherOrFormReader(t *testing.T) {
	fetcher := &fakeFetcher{}
	reader := &fakeFormReader{}
	p := &PreviewClient{fetchers: map[string]applyform.Fetcher{"lever": fetcher}, forms: reader}

	result, err := p.Preview(context.Background(), autoapply.Claimed{Provider: "lever"}, nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !result.Parked || result.Reason != "requires_captcha" {
		t.Errorf("result = %+v, want parked/requires_captcha", result)
	}
	if fetcher.called {
		t.Errorf("fetcher was called, want no schema fetch for a provider that always parks")
	}
}

// Mirrors TestSubmit_ParksHonestlyWhenNoSchemaFetcherIsRegistered: the preview pass runs
// BEFORE Submit ever does (it is what actually reaches a Recruitee-sourced entry first, per
// cmd/auto-apply's own run order), so it needs the same honest park — otherwise a
// Recruitee attempt with no stored form burns the preview's own retry budget toward a
// dead-letter before the candidate ever gets a tailored CV to review.
func TestPreviewClient_ParksHonestlyWhenNoSchemaFetcherIsRegistered(t *testing.T) {
	p := &PreviewClient{fetchers: nil, forms: nil}

	result, err := p.Preview(context.Background(), autoapply.Claimed{Provider: "recruitee"}, nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !result.Parked || result.Reason != reasonSubmissionNotImplemented {
		t.Errorf("result = %+v, want parked/%s", result, reasonSubmissionNotImplemented)
	}
}

func TestPreviewClient_PrefersAStoredFormOverALiveFetchForANonGreenhouseProvider(t *testing.T) {
	fetcher := &fakeFetcher{form: applyform.Form{Fields: []applyform.Field{
		{ID: "should_not_be_used", Type: applyform.TypeText, Required: true},
	}}}
	reader := &fakeFormReader{found: true, form: applyform.Form{Fields: []applyform.Field{
		{ID: "first_name", Label: "First name", Type: applyform.TypeText, Required: true},
	}}}
	p := &PreviewClient{fetchers: map[string]applyform.Fetcher{"ashby": fetcher}, forms: reader}

	result, err := p.Preview(context.Background(), autoapply.Claimed{Provider: "ashby"}, map[string]string{"first_name": "Ada"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if fetcher.called {
		t.Errorf("fetcher was called, want the stored form preferred over a live fetch")
	}
	if len(result.Preview.Fields) != 1 || result.Preview.Fields[0].Value != "Ada" {
		t.Fatalf("preview = %+v, want the stored form's field resolved", result.Preview)
	}
}

func TestPreviewClient_FallsBackToALiveFetchWhenNoFormIsStored(t *testing.T) {
	fetcher := &fakeFetcher{form: applyform.Form{Fields: []applyform.Field{
		{ID: "first_name", Label: "First name", Type: applyform.TypeText, Required: true},
	}}}
	reader := &fakeFormReader{found: false}
	p := &PreviewClient{fetchers: map[string]applyform.Fetcher{"ashby": fetcher}, forms: reader}

	result, err := p.Preview(context.Background(), autoapply.Claimed{Provider: "ashby"}, map[string]string{"first_name": "Ada"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !fetcher.called {
		t.Errorf("fetcher was not called, want a live fetch when no form is stored")
	}
	if len(result.Preview.Fields) != 1 || result.Preview.Fields[0].Value != "Ada" {
		t.Fatalf("preview = %+v, want the fetched form's field resolved", result.Preview)
	}
}
