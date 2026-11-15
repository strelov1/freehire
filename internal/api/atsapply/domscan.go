// Package atsapply drives a headless browser against a job's live application-form page:
// scan what the page renders, reconcile it against the ATS's declared schema
// (internal/applyform's fetchers), resolve the result against the candidate's known answers,
// and fill and submit only when every required question is answered.
package atsapply

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// DOMField is one control found in a rendered application form's DOM, the source of truth for
// what must be filled (a live Greenhouse posting rendered 36 fields against 17 its question API
// declared). Reconcile.go merges it with internal/applyform's API Field for the label and
// option text the DOM often lacks.
type DOMField struct {
	// ID is the identifier this control's own PLATFORM addresses it by: its `id` on Greenhouse,
	// its `name` on Lever, whose inputs carry no id. The layout picks the attribute (see
	// identify); everything downstream works on this one identifier.
	//
	// It is also what the platform expects back on submit, so the attribute is the platform's
	// choice: Lever's résumé upload has id `resume-upload-input` and posts under name `resume`.
	//
	// Empty for a checkbox/radio group, whose members share no id; Name is authoritative there.
	ID string
	// Name is the DOM name attribute. For a checkbox/radio group it is the group's shared
	// key; for everything else it usually equals ID and is kept for that case too.
	Name string
	// Kind is this package's own small vocabulary: "text", "textarea", "select", "file",
	// "checkbox_group". Unlike internal/applyform's FieldType this is DOM-widget shaped,
	// not platform-shaped — it describes what was found, not what the platform calls it.
	Kind string
	// Required is read from the HTML `required` attribute. It is often ABSENT on a field the
	// API schema marks required (Greenhouse's `country`); Reconcile.go closes that gap. This is
	// a faithful report of the DOM alone.
	Required bool
	// Multi is true for a checkbox group, where more than one option may be chosen.
	Multi bool
	// Options are the choices found for a checkbox/radio group, DOM value only — the
	// employer-facing label lives in the API schema and is filled in by Reconcile.go.
	Options []string
}

// ScanForm parses a rendered application page's form into its field inventory, per the
// platform's layout: which element the form renders under and which attribute identifies a
// control. Pure over an HTML string, like internal/ingest/applyform's FromLever.
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
			// A subtree hidden from assistive technology holds nothing a candidate is asked.
			// Greenhouse pairs each custom widget with a `required` proxy input marked this way;
			// the widget writes the real answer through to it. Counting those as questions would
			// leave permanent blank entries in Plan.Unmapped (no id, no name, no label, never
			// resolvable) and keep Plan.FullyResolved() false forever.
			//
			// The whole subtree is skipped: every control below a hidden wrapper is widget
			// plumbing.
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
		// A group is keyed by the name its members share, so byName needs no special case.
		// byID falls back to the id when a group carries no name.
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

// identify returns the identifier this layout addresses a control by, and whether the control
// can be addressed at all.
//
// Under byName a control with no name is DROPPED rather than given fallbackKey's synthetic
// key: that would mint a field that resolves, reports the plan complete, and then cannot be
// typed into. The cost: Reconcile reads only the retained controls, so a required control
// dropped here goes unchecked, and the API's required flag cannot park the attempt on it.
//
// The identifier is what DOMField.ID carries for both ways of addressing; downstream code
// never asks which attribute it came from. Lever's résumé upload and location autocomplete
// have an id that differs from the posted name, so preferring the id there would resolve a
// field Lever has never heard of.
func identify(id, name string, by addressing, order *[]string) (string, bool) {
	if by == byName {
		if name == "" {
			return "", false
		}
		return name, true
	}
	return fallbackKey(id, name, order), true
}

// fallbackKey is id, or name when id is empty, or, when BOTH are empty, a synthetic key unique
// to this scan (order's length only grows). Two anonymous controls must not share the empty
// string: the second would drop the first, and Plan.FullyResolved() could report true over a
// required question never seen.
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
