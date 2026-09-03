package atsapply

import "github.com/strelov1/freehire/internal/ingest/applyform"

// MergedField is one question on the live form, joined from what the DOM renders and what
// the platform's own API schema (internal/applyform.Form) declares about it.
type MergedField struct {
	// ID is the DOM id (or, absent that, name) — the selector Fill uses and, for
	// Greenhouse/Ashby, the same token the platform expects back on submit.
	ID string
	// Label is the API's question text, empty when nothing matched (a DOM-only field, like
	// Greenhouse's `country`, still gets filled — it just has no employer-authored label to
	// show alongside it).
	Label string
	// Kind is DOMField.Kind — always DOM-sourced, since the API describes select OPTIONS,
	// not which widget renders them.
	Kind string
	// Required is the union of the DOM's `required` attribute and the API's own flag — see
	// TestReconcile_RequiredIsTheUnionOfDOMAndAPI for why the DOM attribute alone is not
	// trusted: the spike measured a required react-select field (`country`) rendered
	// without it.
	Required bool
	// Multi carries DOMField.Multi through (a checkbox group takes several answers).
	Multi bool
	// Options are the API's enumerated answers where it declared them (with the value the
	// platform expects), else the DOM's own raw option values for an unmatched
	// checkbox/radio group.
	Options []applyform.Option
}

// domToAPIAlias is the one DOM-id-to-API-id mismatch the 2026-09-02 spike measured on a
// live Greenhouse posting: the rendered location autocomplete carries the DOM id
// `candidate-location`, while the question API calls the same question `location`.
var domToAPIAlias = map[string]string{
	"candidate-location": "location",
}

// Reconcile joins a live DOM scan with the platform's declared schema. The DOM decides what
// exists on the page — an API-declared field the DOM never rendered is dropped, and a
// DOM-rendered field the API never declared is kept regardless — because filling a form is
// about the page in front of the browser, not the platform's description of it.
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
