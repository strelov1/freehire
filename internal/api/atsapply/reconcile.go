package atsapply

import "github.com/strelov1/freehire/internal/ingest/applyform"

// MergedField is one question on the live form, joined from the DOM and the platform's API
// schema (internal/applyform.Form).
type MergedField struct {
	// ID is the DOM id (or, absent that, name) — the selector Fill uses and, for
	// Greenhouse/Ashby, the same token the platform expects back on submit.
	ID string
	// Label is the API's question text; empty for a DOM-only field (Greenhouse's `country`),
	// which is still filled.
	Label string
	// Kind is DOMField.Kind; always DOM-sourced, since the API describes options, not widgets.
	Kind string
	// Required is the union of the DOM `required` attribute and the API flag; the DOM alone
	// is not trusted (see TestReconcile_RequiredIsTheUnionOfDOMAndAPI).
	Required bool
	// Multi carries DOMField.Multi through (a checkbox group takes several answers).
	Multi bool
	// Options are the API's enumerated answers (with the value the platform expects), else
	// the DOM's raw option values for an unmatched checkbox/radio group.
	Options []applyform.Option
}

// questionText is what this field asks, as a person reads it: the employer's label, else
// the field's id.
//
// One function because the review screen (PreviewAnswers) and the answer bank key
// (matchBankAnswerKey) must call a field the same thing. Label is empty for DOM-rendered
// fields the schema never declared (Greenhouse's `country`); when the two diverged, the
// answer was banked under one topic and the resolver folded the other to "", so the field
// parked on every application.
//
// A field with neither label nor id is still refused downstream (answertopic.Of declines
// a fold that yields nothing).
func questionText(f MergedField) string {
	if f.Label != "" {
		return f.Label
	}
	return f.ID
}

// domToAPIAlias maps the one DOM id that differs from its API id on Greenhouse: the
// location autocomplete is `candidate-location` in the DOM and `location` in the API.
var domToAPIAlias = map[string]string{
	"candidate-location": "location",
}

// Reconcile joins a live DOM scan with the platform's declared schema. The DOM decides
// what exists: an API-declared field the DOM never rendered is dropped, a DOM-only field
// is kept, because filling is about the page in front of the browser.
func Reconcile(dom []DOMField, api applyform.Form) []MergedField {
	byAPIID := make(map[string]applyform.Field, len(api.Fields))
	for _, f := range api.Fields {
		byAPIID[f.ID] = f
	}

	out := make([]MergedField, 0, len(dom))
	for _, d := range dom {
		key := d.ID
		if key == "" {
			key = d.Name
		}
		apiKey := key
		if alias, ok := domToAPIAlias[key]; ok {
			apiKey = alias
		}

		m := MergedField{ID: key, Kind: d.Kind, Required: d.Required, Multi: d.Multi}
		if apiField, matched := byAPIID[apiKey]; matched {
			m.Label = apiField.Label
			m.Required = m.Required || apiField.Required
			m.Options = apiField.Options
		} else if len(d.Options) > 0 {
			for _, v := range d.Options {
				m.Options = append(m.Options, applyform.Option{Label: v, Value: v})
			}
		}
		out = append(out, m)
	}
	return out
}
