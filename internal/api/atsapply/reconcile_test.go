package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

func TestReconcile_MatchedFieldGetsTheAPIsLabelAndOptions(t *testing.T) {
	dom := []DOMField{{ID: "question_1", Kind: "select", Required: false}}
	api := applyform.Form{Fields: []applyform.Field{
		{ID: "question_1", Label: "Why do you want to work here?", Required: true,
			Options: []applyform.Option{{Label: "Yes", Value: "1"}}},
	}}

	got := Reconcile(dom, api)

	if len(got) != 1 {
		t.Fatalf("merged = %d fields, want 1", len(got))
	}
	if got[0].Label != "Why do you want to work here?" {
		t.Errorf("label = %q, want the API's", got[0].Label)
	}
	if len(got[0].Options) != 1 {
		t.Errorf("options = %v, want the API's one option", got[0].Options)
	}
}

// The one name mismatch the 2026-09-02 spike measured: the DOM renders `candidate-location`
// where Greenhouse's API calls the same question `location`.
func TestReconcile_AppliesTheKnownDOMToAPIAlias(t *testing.T) {
	dom := []DOMField{{ID: "candidate-location", Kind: "text"}}
	api := applyform.Form{Fields: []applyform.Field{{ID: "location", Label: "Location", Required: true}}}

	got := Reconcile(dom, api)

	if len(got) != 1 || got[0].Label != "Location" {
		t.Fatalf("merged = %+v, want candidate-location matched to the API's location", got)
	}
}

// The spike's core finding: a field the API never declares (country, EEOC) is still real
// because the DOM renders it — it just carries no label beyond its own id.
func TestReconcile_ADOMOnlyFieldIsKeptWithoutAnAPILabel(t *testing.T) {
	dom := []DOMField{{ID: "country", Kind: "text", Required: false}}
	api := applyform.Form{}

	got := Reconcile(dom, api)

	if len(got) != 1 || got[0].ID != "country" || got[0].Label != "" {
		t.Fatalf("merged = %+v, want the DOM-only field kept with no API label", got)
	}
}

// DOM is authoritative for existence: an API-declared field the DOM never rendered (the
// EEOC block on boards that opted out, e.g.) must not appear — filling it would write into
// a control that isn't on the page.
func TestReconcile_AnAPIOnlyFieldNeverRenderedIsDropped(t *testing.T) {
	dom := []DOMField{}
	api := applyform.Form{Fields: []applyform.Field{{ID: "gender", Label: "Gender", Required: false}}}

	got := Reconcile(dom, api)

	if len(got) != 0 {
		t.Errorf("merged = %+v, want nothing — the DOM never rendered it", got)
	}
}

// The required flag is a union, not DOM-only: the spike measured `country` rendered with no
// HTML `required` attribute (react-select controls frequently omit it) while Greenhouse's own
// API declares it required. Trusting the DOM attribute alone here would silently let a
// required question through unanswered.
func TestReconcile_RequiredIsTheUnionOfDOMAndAPI(t *testing.T) {
	dom := []DOMField{{ID: "country", Kind: "text", Required: false}}
	api := applyform.Form{Fields: []applyform.Field{{ID: "country", Label: "Country", Required: true}}}

	got := Reconcile(dom, api)

	if len(got) != 1 || !got[0].Required {
		t.Fatalf("merged = %+v, want Required=true from the API even though the DOM attribute was absent", got)
	}
}
