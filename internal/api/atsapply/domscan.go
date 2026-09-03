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
	// ID is the element's own id attribute — the selector this package fills by, and (for
	// Greenhouse) the same identifier the platform expects back on submit. Empty for a
	// checkbox/radio group, whose members share no single id; Name is authoritative there.
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

// ScanGreenhouseForm parses a rendered Greenhouse application page's `#application-form`
// into its field inventory. Pure function over an HTML string — no browser, no network —
// mirroring internal/applyform's own FromLever, which parses markup the same way for a
// platform whose form has no separate question API.
func ScanGreenhouseForm(pageHTML string) ([]DOMField, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, fmt.Errorf("parse page: %w", err)
	}
	form := findByID(doc, "application-form")
	if form == nil {
		return nil, fmt.Errorf("no #application-form on the page")
	}
	return scanControls(form), nil
}

// scanControls walks a form node's input/select/textarea controls into DOMFields, grouping
// checkbox/radio siblings that share a name into one field.
func scanControls(form *html.Node) []DOMField {
	var order []string // name/id order, so the returned slice matches document order
	groups := map[string]*DOMField{}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "input":
				scanInput(n, &order, groups)
			case "textarea":
				scanSimple(n, "textarea", &order, groups)
			case "select":
				scanSimple(n, "select", &order, groups)
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

func scanInput(n *html.Node, order *[]string, groups map[string]*DOMField) {
	typ := attr(n, "type")
	if typ == "hidden" {
		// Platform-filled, never a candidate answer — see the package doc.
		return
	}
	id := attr(n, "id")
	name := attr(n, "name")

	if typ == "checkbox" || typ == "radio" {
		key := name
		if key == "" {
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
	key := fallbackKey(id, name, order)
	if _, ok := groups[key]; ok {
		return // a real duplicate of an already-keyed (id or name) field — stay idempotent
	}
	groups[key] = &DOMField{ID: id, Name: name, Kind: kind, Required: hasAttr(n, "required")}
	*order = append(*order, key)
}

func scanSimple(n *html.Node, kind string, order *[]string, groups map[string]*DOMField) {
	id := attr(n, "id")
	name := attr(n, "name")
	key := fallbackKey(id, name, order)
	groups[key] = &DOMField{ID: id, Name: name, Kind: kind, Required: hasAttr(n, "required")}
	*order = append(*order, key)
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
