package atsapply

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// Lever always parks on its captcha before any fetcher or browser is touched — a nil
// fetchers map would panic if this short-circuit were ever removed, which is deliberate:
// it proves nothing downstream runs for this provider.
func TestSubmit_LeverAlwaysParksOnCaptchaWithoutTouchingFetchersOrBrowser(t *testing.T) {
	c := &Client{fetchers: nil}

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "lever"}, nil)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || result.Reason != "requires_captcha" {
		t.Errorf("result = %+v, want parked/requires_captcha", result)
	}
}

// Recruitee (and any other source jobs.source can carry that internal/ingest/applyform
// never registered a Fetcher for) has no schema fetcher at all — before this fix,
// fetchSchema's plain error bubbled up as an ordinary retryable Fail, so an attempt for one
// of these silently burned the whole retry budget toward a dead-letter instead of parking
// honestly like Ashby/Workable's own "submission not yet implemented" outcome.
func TestSubmit_ParksHonestlyWhenNoSchemaFetcherIsRegistered(t *testing.T) {
	c := &Client{fetchers: nil}

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "recruitee"}, nil)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || result.Reason != reasonSubmissionNotImplemented {
		t.Errorf("result = %+v, want parked/%s", result, reasonSubmissionNotImplemented)
	}
}

// A provider with no live fetcher (Recruitee) reaches field resolution using a stored form
// instead of parking immediately — an incomplete resolution proves the stored form was
// actually read and used: Unmapped names the real missing field, not the generic
// reasonSubmissionNotImplemented a provider with no schema at all would report.
func TestSubmit_UsesAStoredFormWhenNoLiveFetcherIsRegistered(t *testing.T) {
	reader := &fakeFormReader{found: true, form: applyform.Form{Fields: []applyform.Field{
		{ID: "first_name", Label: "First name", Type: applyform.TypeText, Required: true},
	}}}
	c := &Client{fetchers: nil, forms: reader}

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "recruitee"}, map[string]string{})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || len(result.Unmapped) != 1 || result.Unmapped[0].ID != "first_name" {
		t.Fatalf("result = %+v, want parked with first_name named as unmapped — proves the stored form's own field reached resolution", result)
	}
}

// The stored-form fallback still parks honestly when nothing is stored either — same
// outcome as no fetcher at all, just reached through a live c.forms that found nothing
// rather than a nil one.
func TestSubmit_ParksHonestlyWhenAFormReaderFindsNothingStored(t *testing.T) {
	reader := &fakeFormReader{found: false}
	c := &Client{fetchers: nil, forms: reader}

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "recruitee"}, nil)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || result.Reason != reasonSubmissionNotImplemented {
		t.Errorf("result = %+v, want parked/%s", result, reasonSubmissionNotImplemented)
	}
}

// A provider that already has a live fetcher must keep using it — apply_forms holds a row
// for Ashby too (cmd/capture-apply-form captures it for the job page's own display), so a
// stored-form-first order would silently prefer that possibly-stale row over a fresh fetch
// for a real submission. Found by design review before this shipped: the first draft of
// this change mirrored PreviewClient.schemaFor's storage-first order, which is right for a
// cheap preview but wrong here.
func TestSubmit_AProviderWithALiveFetcherIgnoresAStoredForm(t *testing.T) {
	fetcher := &fakeFetcher{form: applyform.Form{Fields: []applyform.Field{
		{ID: "from_live_fetch", Label: "From live fetch", Type: applyform.TypeText, Required: true},
	}}}
	reader := &fakeFormReader{found: true, form: applyform.Form{Fields: []applyform.Field{
		{ID: "from_stored_form", Label: "From stored form", Type: applyform.TypeText, Required: true},
	}}}
	c := &Client{fetchers: map[string]applyform.Fetcher{"ashby": fetcher}, forms: reader}

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "ashby"}, map[string]string{})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !fetcher.called {
		t.Error("live fetcher was not called, want it preferred over the stored form for a provider that has one")
	}
	if len(result.Unmapped) != 1 || result.Unmapped[0].ID != "from_live_fetch" {
		t.Fatalf("unmapped = %+v, want the live-fetched field named, not the stored form's", result.Unmapped)
	}
}

// A stored-form resolution that fully resolves still parks — Recruitee has no fill path
// (fillProviders) and no browser-use fallback (browserUseProviders), so a complete plan is
// exactly as unsubmittable as an incomplete one, just for a different reason.
func TestSubmit_AFullyResolvedStoredFormStillParksWithNoSubmitPath(t *testing.T) {
	reader := &fakeFormReader{found: true, form: applyform.Form{Fields: []applyform.Field{
		{ID: "first_name", Label: "First name", Type: applyform.TypeText, Required: true},
	}}}
	c := &Client{fetchers: nil, forms: reader}

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "recruitee"}, map[string]string{"first_name": "Ada"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || result.Reason != reasonSubmissionNotImplemented {
		t.Fatalf("result = %+v, want parked/%s — fully resolved but still no submit path for this provider", result, reasonSubmissionNotImplemented)
	}
	if len(result.Unmapped) != 0 {
		t.Errorf("unmapped = %+v, want none — every required field resolved", result.Unmapped)
	}
}

func TestUnscannableFormResult_MapsBothReasonsToParked(t *testing.T) {
	for _, reason := range []unscannableFormReason{reasonCaptchaProtected, reasonUnrecognizedLayout} {
		result, parked := unscannableFormResult(&unscannableFormError{reason: reason})
		if !parked {
			t.Fatalf("unscannableFormResult(%q): parked = false, want true", reason)
		}
		if result.Status != autoapply.StatusParked || result.Reason != string(reason) {
			t.Errorf("unscannableFormResult(%q) = %+v, want parked/%s", reason, result, reason)
		}
	}
}

func TestUnscannableFormResult_LeavesAGenuineErrorUnparked(t *testing.T) {
	result, parked := unscannableFormResult(errors.New("net/http: TLS handshake timeout"))
	if parked {
		t.Errorf("unscannableFormResult(plain error) = %+v, parked = true, want an ordinary retryable error", result)
	}
}

func TestMergedFromAPIOnly_SkipsHiddenAndInfoFields(t *testing.T) {
	api := applyform.Form{Fields: []applyform.Field{
		{ID: "keep", Type: applyform.TypeText, Required: true},
		{ID: "gh_src", Type: applyform.TypeHidden},
		{ID: "blurb", Type: applyform.TypeInfo},
	}}

	got := mergedFromAPIOnly(api)

	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("merged = %+v, want only the one answerable field", got)
	}
}

// recordingAtomReader records every ListAtoms call, so a test can prove drafting's
// (expensive: a real grounding read plus an LLM call) path was never entered.
type recordingAtomReader struct {
	calls int
}

func (r *recordingAtomReader) ListAtoms(context.Context, int64) ([]experience.Atom, error) {
	r.calls++
	return nil, nil
}

// Found by code review: Client.resolve ran the full drafting path — a grounding-context DB
// read plus a real, budget-attributed LLM call — for EVERY provider, even Ashby, whose
// result Submit unconditionally discards two lines later ("submission not yet implemented
// for this provider"). Every Ashby attempt with an unmapped field paid for an LLM call
// whose answer could never be used.
func TestClientResolve_SkipsDraftingForAProviderSubmitCannotHandle(t *testing.T) {
	atoms := &recordingAtomReader{}
	c := &Client{atoms: atoms}

	fields := []MergedField{{ID: "q1", Label: "Where did you hear about us?", Kind: "text", Required: true}}
	plan, err := c.resolve(context.Background(), autoapply.Claimed{Provider: "ashby"}, fields, map[string]string{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if atoms.calls != 0 {
		t.Errorf("ListAtoms called %d times, want 0 — Ashby can never reach fillAndSubmit, so drafting for it is pure waste", atoms.calls)
	}
	if len(plan.Unmapped) != 1 {
		t.Fatalf("unmapped = %+v, want the deterministic (undrafted) result", plan.Unmapped)
	}
}

func TestDomKindFor_MapsEveryFieldType(t *testing.T) {
	cases := map[applyform.FieldType]string{
		applyform.TypeText:        "text",
		applyform.TypeTextarea:    "textarea",
		applyform.TypeSelect:      "select",
		applyform.TypeMultiSelect: "select",
		applyform.TypeFile:        "file",
		applyform.TypeBoolean:     "checkbox_group",
		applyform.TypeDate:        "text",
		applyform.TypeNumber:      "text",
	}
	for ft, want := range cases {
		if got := domKindFor(ft); got != want {
			t.Errorf("domKindFor(%q) = %q, want %q", ft, got, want)
		}
	}
}
