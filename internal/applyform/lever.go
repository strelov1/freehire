package applyform

import (
	"strings"

	"golang.org/x/net/html"
)

// Lever is the first captured platform that publishes its form as a rendered page
// rather than as structured data, so this file parses markup where the others decode
// JSON. It reads only `li.application-question` blocks and ignores the rest of the
// document, which is most of a 731 KB page and all of it noise.

// requiredGlyph is what Lever appends to a required question's label. It states the
// requirement, which belongs on the control as a flag; left in place it would put
// punctuation in the middle of every required question on the page.
const requiredGlyph = "✱"

// FromLever captures the application form rendered on a Lever apply page.
func FromLever(doc *html.Node) Form {
	form := Form{Provider: "lever"}
	for _, block := range questionBlocks(doc) {
		form.Fields = append(form.Fields, leverBlock(block)...)
	}
	return form
}

// questionBlocks collects the per-question list items.
func questionBlocks(n *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "application-question") {
			out = append(out, n)
			// A question block does not nest another, and its own <li>s are the radio
			// alternatives — descending would collect those as questions.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

// leverBlock turns one question block into the controls it holds — usually one.
//
// Controls are grouped by submit name rather than taken one by one, and that grouping
// is the reason this cannot be a flat walk: a radio group is several inputs sharing a
// name, and read individually each alternative would become a question of its own.
func leverBlock(block *html.Node) []Field {
	label := blockLabel(block)

	var order []string
	byName := map[string][]*html.Node{}
	for _, c := range controls(block) {
		name := attr(c, "name")
		if name == "" {
			continue
		}
		if _, seen := byName[name]; !seen {
			order = append(order, name)
		}
		byName[name] = append(byName[name], c)
	}

	var fields []Field
	for _, name := range order {
		group := byName[name]
		f := Field{
			ID:       name,
			Label:    label,
			Required: anyRequired(group),
		}
		f.Type, f.RawType, f.Options = leverControl(group)
		fields = append(fields, f)
	}
	return fields
}

// leverControl reads a group of same-named controls into a kind and its options.
func leverControl(group []*html.Node) (FieldType, string, []Option) {
	// A radio group is the multi-input case: each input carries its own submit value and
	// the text a candidate reads sits in a sibling span, so the two halves of an option
	// come from two different places.
	if len(group) > 1 && attr(group[0], "type") == "radio" {
		var opts []Option
		for _, in := range group {
			opts = append(opts, Option{Label: alternativeText(in), Value: attr(in, "value")})
		}
		return TypeSelect, "radio", opts
	}

	node := group[0]
	switch node.Data {
	case "textarea":
		return TypeTextarea, "textarea", nil
	case "select":
		return TypeSelect, "select", selectOptions(node)
	}

	// A checkbox may arrive paired with a hidden input carrying its unchecked value.
	// That pair is one control, so the visible half decides the kind.
	kind := attr(node, "type")
	for _, c := range group {
		if t := attr(c, "type"); t != "hidden" {
			kind = t
			break
		}
	}
	switch kind {
	case "checkbox":
		return TypeBoolean, "checkbox", nil
	case "file":
		return TypeFile, "file", nil
	case "hidden":
		return TypeHidden, "hidden", nil
	case "radio":
		// A lone radio is still a choice, just one with a single alternative.
		return TypeSelect, "radio", []Option{{Label: alternativeText(node), Value: attr(node, "value")}}
	default:
		// text, email, tel, url — all one-line text entry. The vocabulary names the kind
		// of control, and the platform's own word survives in RawType.
		return TypeText, kind, nil
	}
}

// blockLabel reads the question as the employer wrote it, with the required marker
// removed and whitespace collapsed — the markup lays the label out across several nodes.
//
// A consent block carries no application-label at all: the text a candidate reads sits
// inside the <label> beside the checkbox. The wrapper around it varies by tenant — one
// board puts it in a `p.application-answer-alternative`, another in a bare `span` — so
// the fallback reads the enclosing <label> itself, which is what makes that text a label
// in the first place and does not depend on how the employer styled it.
//
// Without a fallback the control was captured unnamed and reached production as a stray
// comma at the end of the standard-fields line.
func blockLabel(block *html.Node) string {
	for _, n := range descendants(block) {
		if hasClass(n, "application-label") {
			return tidyLabel(textOf(n))
		}
	}
	for _, n := range descendants(block) {
		if n.Data == "label" {
			if text := tidyLabel(textOf(n)); text != "" {
				return text
			}
		}
	}
	return ""
}

// tidyLabel drops the required marker and collapses the whitespace the markup used for
// layout rather than for the sentence.
func tidyLabel(text string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(text, requiredGlyph, " ")), " ")
}

// controls returns the block's form controls in document order.
func controls(block *html.Node) []*html.Node {
	var out []*html.Node
	for _, n := range descendants(block) {
		switch n.Data {
		case "input", "select", "textarea":
			out = append(out, n)
		}
	}
	return out
}

// selectOptions reads a dropdown's answers. An option with no value is the "Select…"
// placeholder, which is a prompt rather than an answer.
func selectOptions(node *html.Node) []Option {
	var opts []Option
	for _, n := range descendants(node) {
		if n.Data != "option" {
			continue
		}
		value := attr(n, "value")
		if value == "" {
			continue
		}
		opts = append(opts, Option{Label: strings.TrimSpace(textOf(n)), Value: value})
	}
	return opts
}

// alternativeText reads the text presented beside a radio input, which is what a
// candidate reads — the input itself carries only the submit value.
func alternativeText(in *html.Node) string {
	for p := in.Parent; p != nil; p = p.Parent {
		for _, n := range descendants(p) {
			if hasClass(n, "application-answer-alternative") {
				return strings.TrimSpace(textOf(n))
			}
		}
		// Stop at the alternative's own list item; going further would read a sibling's.
		if p.Data == "li" {
			break
		}
	}
	return attr(in, "value")
}

func anyRequired(group []*html.Node) bool {
	for _, n := range group {
		if attr(n, "required") != "" {
			return true
		}
	}
	return false
}

func descendants(n *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}
