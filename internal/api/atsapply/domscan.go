// Package atsapply drives a headless browser against a job's live application-form page:
// scan what the page actually renders, reconcile it against the ATS's own declared schema
// (internal/applyform's existing fetchers, reused rather than re-derived), resolve the
// result against a candidate's known answers, and fill and submit only when every required
// question is answered. See openspec/changes/auto-apply-worker/design.md for why this
// package exists as chromedp (Go, in-process) rather than a separate Python sidecar.
package atsapply

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// DOMField is one control found in a rendered application form's DOM — the source of truth
// for what must be filled, per the 2026-09-02 spike: a live Greenhouse posting rendered 36
// fields against 17 the platform's own question API declared. Reconcile.go merges this with
// internal/applyform's API-declared Field for the label/option text the DOM alone often
// lacks.
type DOMField struct {
	// ID is the identifier this control's own PLATFORM addresses it by — its `id` on
	// Greenhouse, its `name` on Lever, whose inputs carry no id at all. Which attribute
	// that is comes from the layout (see identify); everything downstream works on this
	// one identifier and never asks where it came from.
	//
	// It is also what the platform expects back on submit, which is why the attribute has
	// to be the platform's choice rather than ours: Lever's résumé upload carries the id
	// `resume-upload-input` and posts under the name `resume`.
	//
	// Empty for a checkbox/radio group, whose members share no single id; Name is
	// authoritative there. That predates the layout and is unchanged by it.
	ID string
	// Name is the DOM name attribute. For a checkbox/radio group it is the group's shared
	// key; for everything else it usually equals ID and is kept for that case too.
	Name string
	// Kind is this package's own small vocabulary: "text", "textarea", "select", "file",
	// "checkbox_group". Unlike internal/applyform's FieldType this is DOM-widget shaped,
	// not platform-shaped — it describes what was found, not what the platform calls it.
	Kind string
	// Required is read from the HTML `required` attribute. It is frequently ABSENT on a
	// field the platform's own API schema marks required (Greenhouse's `country` in the
	// spike) — Reconcile.go is where that gap gets closed, not here: this is a faithful
	// report of the DOM alone.
	Required bool
	// Multi is true for a checkbox group, where more than one option may be chosen.
	Multi bool
	// Options are the choices found for a checkbox/radio group, DOM value only — the
	// employer-facing label lives in the API schema and is filled in by Reconcile.go.
	Options []string
}

// ScanForm parses a rendered application page's form into its field inventory, per the
// platform's own layout: which element the form renders under, and which attribute
// identifies a control on it. Pure function over an HTML string — no browser, no network —
// mirroring internal/ingest/applyform's own FromLever, which parses markup the same way for
// a platform whose form has no separate question API.
func ScanForm(pageHTML string, layout formLayout) ([]DOMField, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, fmt.Errorf("parse page: %w", err)
	}
	form := findByID(doc, layout.formSelector)
	if form == nil {
		return nil, fmt.Errorf("no #%s on the page", layout.formSelector)
	}
	return scanControls(form, layout.addressBy), nil
}

// scanControls walks a form node's input/select/textarea controls into DOMFields, grouping
// checkbox/radio siblings that share a name into one field.
func scanControls(form *html.Node, by addressing) []DOMField {
	var order []string // name/id order, so the returned slice matches document order
	groups := map[string]*DOMField{}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// A subtree the page has explicitly hidden from assistive technology holds
			// nothing a candidate is being asked. Greenhouse's own form components pair
			// each custom widget with a `required` proxy input marked this way, so the
			// browser's native validation fires for a control that is not a native form
			// control; the widget writes the real answer through to it.
			//
			// Counting those as questions is not cosmetic. They carry no id and no name,
			// so they can never reconcile against the platform's schema, never carry a
			// label, and never resolve from a candidate's answers — they sat in
			// Plan.Unmapped permanently, which showed the candidate blank lines where
			// questions should be and kept Plan.FullyResolved() false forever. A vanilla
			// Greenhouse posting rendered four, so no such attempt could ever have been
			// submitted, however complete the profile (freehire, 2026-09-08).
			//
			// The whole subtree is skipped, not just this node: aria-hidden hides what is
			// inside it too, and a wrapper carrying it means every control below is a
			// widget's plumbing rather than its question.
			if attr(n, "aria-hidden") == "true" {
				return
			}
			switch n.Data {
			case "input":
				scanInput(n, by, &order, groups)
			case "textarea":
				scanSimple(n, "textarea", by, &order, groups)
			case "select":
				scanSimple(n, "select", by, &order, groups)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)

	out := make([]DOMField, 0, len(order))
	for _, key := range order {
		out = append(out, *groups[key])
	}
	return out
}

func scanInput(n *html.Node, by addressing, order *[]string, groups map[string]*DOMField) {
	typ := attr(n, "type")
	if typ == "hidden" {
		// Platform-filled, never a candidate answer — see the package doc.
		return
	}
	id := attr(n, "id")
	name := attr(n, "name")

	if typ == "checkbox" || typ == "radio" {
		// A group is keyed by the name its members share — that is what makes them one
		// question — so byName needs no special case here. byID still falls back to the
		// id when a group carries no name at all.
		key := name
		if key == "" {
			if by == byName {
				return
			}
			key = id
		}
		g, ok := groups[key]
		if !ok {
			g = &DOMField{Name: name, Kind: "checkbox_group", Multi: typ == "checkbox"}
			groups[key] = g
			*order = append(*order, key)
		}
		g.Options = append(g.Options, attr(n, "value"))
		if hasAttr(n, "required") {
			g.Required = true
		}
		return
	}

	kind := "text"
	if typ == "file" {
		kind = "file"
	}
	key, addressable := identify(id, name, by, order)
	if !addressable {
		return
	}
	if _, ok := groups[key]; ok {
		return // a real duplicate of an already-keyed field — stay idempotent
	}
	groups[key] = &DOMField{ID: key, Name: name, Kind: kind, Required: hasAttr(n, "required")}
	*order = append(*order, key)
}

func scanSimple(n *html.Node, kind string, by addressing, order *[]string, groups map[string]*DOMField) {
	id := attr(n, "id")
	name := attr(n, "name")
	key, addressable := identify(id, name, by, order)
	if !addressable {
		return
	}
	groups[key] = &DOMField{ID: key, Name: name, Kind: kind, Required: hasAttr(n, "required")}
	*order = append(*order, key)
}

// identify returns the identifier this layout addresses a control by, and whether the
// control can be addressed at all.
//
// Under byName a control with no name is DROPPED rather than given fallbackKey's synthetic
// key. That key exists so two anonymous controls do not collide in one scan; under byName it
// would instead mint a field that resolves, reports the plan complete, and then cannot be
// typed into — the silent last-step failure the addressing setting exists to prevent. What
// remains able to declare such a control required is the platform's own schema, and an
// attempt for it parks, which is honest.
//
// The returned identifier is what DOMField.ID carries, for BOTH addressings: everything
// downstream — Reconcile, Resolve, the plan, the answer bank's topics — works on one
// identifier, and which attribute it came from is this function's business alone. Lever's
// résumé upload and location autocomplete both carry an id that differs from the name the
// platform posts under, so preferring the id there would resolve a field Lever has never
// heard of.
func identify(id, name string, by addressing, order *[]string) (string, bool) {
	if by == byName {
		if name == "" {
			return "", false
		}
		return name, true
	}
	return fallbackKey(id, name, order), true
}

// fallbackKey is id, or name when id is empty, or — when BOTH are empty — a synthetic key
// unique to this scan (order's current length, which only ever grows). Found by code
// review: two id-less/name-less controls on one page both fell back to the bare empty
// string, so the second silently collided with and dropped the first — a required field
// that vanished from the scan entirely rather than merely being unfillable, which let
// Plan.FullyResolved() report true while a real required question had never been seen.
func fallbackKey(id, name string, order *[]string) string {
	if id != "" {
		return id
	}
	if name != "" {
		return name
	}
	return fmt.Sprintf("_unnamed_%d", len(*order))
}

// findByID returns the first descendant element with the given id, or nil.
func findByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode && attr(n, "id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
