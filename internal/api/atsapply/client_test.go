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
