// Package reqextract derives a posting's own stated requirements from its
// description markup, with no model call. It is the second producer of the
// enrichment contract's `requirements` field: the LLM reads requirements stated as
// prose, this reads those stated as a list under a heading, and the two union.
//
// Extraction is dictionary-gated: a heading outside the vocabulary yields nothing, and
// there is deliberately no fallback that guesses which list is the requirements list
// (the next most common list is benefits, and perks read as requirements are worse
// than nothing).
//
// The bound is enrich.BoundRequirements, shared so the two producers cannot drift.
package reqextract

import (
	"encoding/json"
	"strings"

	"github.com/mozillazg/go-unidecode"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/strelov1/freehire/internal/ai/enrich"
)

// maxHeadingRunes bounds how long a candidate heading may be. Only inline elements
// (`p`, `strong`, `b`, `div`) need it, to tell "The requirements for this role are
// flexible…" (a sentence) from a title. h1–h6 are not length-checked.
const maxHeadingRunes = 60

// preferredHeadings and requiredHeadings are the controlled vocabulary the
// derivation is gated on, normalized the way normalizeHeading normalizes a node's
// text. A heading matches when its normalized text EQUALS or BEGINS with a phrase.
//
// preferredHeadings is consulted first and must stay first: "qualifications
// (preferred)" matches both lists, and the optional reading is the safe one, since
// overstating a nice-to-have as required is undetectable.
var preferredHeadings = []string{
	"nice to have",
	"nice to haves",
	"nice if you have",
	"preferred",
	"preferred qualifications",
	"preferred skills",
	"bonus",
	"bonus points",
	"desirable",
	"desired",
	"good to have",
	"pluses",
	"would be a plus",
	"a plus",
	"it would be great if you",
	// Hungarian: "Előnyt jelent" — "counts as an advantage" — is how the Hungarian
	// boards head their nice-to-have section. Spelled here the way normalizeHeading
	// transliterates it.
	"elonyt jelent",
	"elony",
}

// closingHeadings are the section titles that END a requirements section without
// starting one. Structure alone cannot tell an inline heading (a bolded paragraph)
// from a lead-in like `<p>You will need:</p>`, so an inline element must be
// RECOGNIZED to close a section; an h1–h6 outside the vocabulary closes on its tag.
// These mostly name the benefits section, which must never be read as requirements.
// An unrecognized inline line leaves the section open.
var closingHeadings = []string{
	"about us",
	"about the company",
	"about the role",
	"about the team",
	"benefits",
	"benefits and perks",
	"compensation",
	"compensation and benefits",
	"how to apply",
	"interview process",
	"next steps",
	"our offer",
	"perks",
	"perks and benefits",
	"salary",
	"the process",
	"what we offer",
	"what you get",
	"what you will get",
	"why join us",
	"amit ajanlunk", // Hungarian: "Amit ajánlunk" — "what we offer"
	"amit kinalunk", // Hungarian: "Amit kínálunk" — "what we provide"
	// Russian: "Мы предлагаем" — "we offer", transliterated the way normalizeHeading
	// spells it. See reqextract/AGENTS.md's Limitations.
	"my predlagaem",
	// Russian: "Условия" — "terms"/"conditions", the closing section after benefits.
	"usloviia",
}

var requiredHeadings = []string{
	"requirements",
	"elvarasok", // Hungarian: "Elvárások" — "requirements"
	"requirement",
	"required",
	"required qualifications",
	"required skills",
	"qualifications",
	"minimum qualifications",
	"basic qualifications",
	"must have",
	"must haves",
	// Contractions reach here with the apostrophe removed (see normalizeHeading), so
	// the vocabulary spells them the way they arrive: "what you'll need" → "what
	// youll need".
	"what youll need",
	"what you will need",
	"what you need",
	"what were looking for",
	"what we are looking for",
	"what we expect",
	"who you are",
	"your profile",
	"your qualifications",
	"skills and experience",
	"experience required",
	"we are looking for",
	// Russian: "Требования" — "requirements".
	"trebovaniia",
	// Russian: "Мы ждем от вас" — "what we expect from you". Two spellings: go-unidecode
	// turns "ё" into "io", so "Мы ждём от вас" is "my zhdiom ot vas" and "Мы ждем" is
	// "my zhdem ot vas".
	"my zhdem ot vas",
	"my zhdiom ot vas",
}

// Derive returns the requirements a posting's description states as a list under a
// recognized heading, in document order, bounded by the enrichment contract's own
// ceiling. It returns nil when the description has no such heading — which is the
// majority of postings and is not an error.
func Derive(descriptionHTML string) []enrich.Requirement {
	if strings.TrimSpace(descriptionHTML) == "" {
		return nil
	}
	doc, err := xhtml.Parse(strings.NewReader(descriptionHTML))
	if err != nil {
		// x/net/html does not reject malformed markup, so this is effectively
		// unreachable; yield nothing rather than fail a crawl.
		return nil
	}

	var found []enrich.Requirement
	// priority is the open section's priority ("" when none). A section closes on taking
	// its list, an unrecognized structural heading, or a table.
	priority := ""
	// pending buffers candidate item texts collected since the section opened while no
	// list has been found: too-long text blocks (prose) and short inline blocks the
	// vocabulary does not recognize (lead-ins, spacers, or short items). A list found
	// next discards prose or just clears lead-ins; reaching the section's natural end
	// with two or more commits them as items, for postings that state each requirement
	// as its own paragraph instead of a <ul>. pendingHasProse marks whether any
	// buffered entry was the too-long kind, which a list found afterward is read by.
	var pending []string
	var pendingHasProse bool

	resetPending := func() {
		pending = nil
		pendingHasProse = false
	}

	// flushPending commits the buffered paragraphs under the given priority when there
	// are two or more (a p-per-item section; one is an ambiguous sentence). Called when
	// a section ends outside the list case: a heading transition, a table, or the end.
	flushPending := func(forPriority string) {
		if len(pending) >= 2 {
			for _, text := range pending {
				found = append(found, enrich.Requirement{Text: text, Priority: forPriority})
			}
		}
		resetPending()
	}

	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch {
			// A container holding the list is a wrapper, not a heading, however short
			// its text; treating it as one would skip the whole subtree.
			case wrapsAList(n):

			case isHeadingCandidate(n):
				next, recognized := headingDecision(n, priority)
				if recognized {
					flushPending(priority)
					priority = next
					return // the heading's own text is never an item
				}
				// Not recognized: buffered, since a run of these may become items.
				if priority != "" {
					if text := textOf(n); text != "" {
						pending = append(pending, text)
					}
				}
				return

			case priority != "" && (n.DataAtom == atom.Ul || n.DataAtom == atom.Ol):
				if pendingHasProse {
					// Prose preceded this list, so the list belongs to whatever
					// follows, not to this section.
					resetPending()
					priority = ""
					return
				}
				resetPending() // any buffered lead-in lines were never content
				for _, text := range listItems(n) {
					found = append(found, enrich.Requirement{Text: text, Priority: priority})
				}
				priority = "" // only the FIRST list after a heading is the section's
				return        // nested lists were consumed by listItems

			case n.DataAtom == atom.Table:
				flushPending(priority)
				priority = ""

			// Too long to be a title, so a paragraph. Buffered rather than closing the
			// section: it may be the section's first item, which only what follows
			// (more paragraphs, a list, the section's end) can tell.
			case isTextBlock(n):
				if priority != "" {
					if text := textOf(n); text != "" {
						pending = append(pending, text)
						pendingHasProse = true
					}
				}
				// textOf already read every descendant; do not walk them again, or a
				// nested <strong>/<b>/<div> would be buffered twice.
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	flushPending(priority)

	return enrich.BoundRequirements(found)
}

// isTextBlock reports whether an element is one of the four used for both section
// titles and paragraphs; which one it is depends on length (see isHeadingCandidate).
func isTextBlock(n *xhtml.Node) bool {
	switch n.DataAtom {
	case atom.P, atom.Strong, atom.B, atom.Div:
		return true
	default:
		return false
	}
}

// wrapsAList reports whether a text block that could otherwise pass for a heading is
// really a container holding the list, as in `<div><h3>Requirements</h3><ul>…</ul></div>`.
func wrapsAList(n *xhtml.Node) bool {
	return isTextBlock(n) && containsList(n)
}

// containsList reports whether any descendant is a list or a table.
func containsList(n *xhtml.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode {
			switch c.DataAtom {
			case atom.Ul, atom.Ol, atom.Table:
				return true
			}
		}
		if containsList(c) {
			return true
		}
	}
	return false
}

// isHeadingCandidate reports whether a node is in a position to carry a section title:
// an h1–h6, or a text block short enough to be a line rather than a paragraph. What it
// does to the open section is headingDecision's call.
func isHeadingCandidate(n *xhtml.Node) bool {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return true
	default:
		return isTextBlock(n) && len([]rune(textOf(n))) <= maxHeadingRunes
	}
}

// headingDecision returns the section priority in force after this node, given the one
// in force before it, and whether the node was RECOGNIZED as a section title at all.
//
// Derive ignores the second value. [MaskPreferred] needs it: an unrecognized short line
// inside a preferred section is that section's CONTENT, and treating it as a title would
// leave its words unmasked.
//
//   - In the requirements vocabulary: OPENS a section at that priority.
//   - In closingHeadings, or a structural heading (h1–h6) outside every vocabulary:
//     CLOSES the open section.
//   - Any other inline text LEAVES the section as it was (spacers like `<p></p>` and
//     lead-ins like `<p>You will need:</p>`; closing on those loses real lists).
//
// An h1–h6 announces itself by tag so an unrecognized one can close; an inline
// `<p>Benefits:</p>` has the same shape as a lead-in, so only a vocabulary tells them apart.
// Prose never reaches here: text longer than maxHeadingRunes is not a candidate.
func headingDecision(n *xhtml.Node, open string) (string, bool) {
	heading := normalizeHeading(textOf(n))
	if matches(heading, preferredHeadings) {
		return "preferred", true
	}
	if matches(heading, requiredHeadings) {
		return "required", true
	}
	if matches(heading, closingHeadings) {
		return "", true
	}
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return "", true
	default:
		return open, false
	}
}

// headingTail is the closed set of words that may follow a vocabulary phrase without
// changing what the heading is: connectives, and nouns that name the same section twice
// ("Requirements & Qualifications"). A word outside the set means the line names
// something else, which separates "Required Qualifications & Skills" (a title) from
// "MUST HAVE MORNING/DAYTIME AVAILABILITY" (a benefits sentence).
var headingTail = map[string]bool{
	"a": true, "and": true, "for": true, "of": true, "or": true, "the": true,
	"this": true, "we": true, "you": true, "your": true,
	"attributes": true, "background": true, "candidate": true, "competences": true,
	"competencies": true, "criteria": true, "essential": true, "expertise": true,
	"experience": true, "have": true, "haves": true, "ideal": true, "job": true,
	"knowledge": true, "minimum": true, "position": true, "preferred": true,
	"profile": true, "qualification": true, "qualifications": true, "required": true,
	"requirement": true, "requirements": true, "role": true, "skill": true,
	"skills": true, "successful": true,
}

// matches reports whether a normalized heading is one of the vocabulary's phrases,
// either exactly or followed only by headingTail words.
//
// A bare prefix match would also admit "Must have morning availability", a sentence;
// requiring the remainder to be tail vocabulary keeps statements from opening a section.
func matches(heading string, vocabulary []string) bool {
	for _, phrase := range vocabulary {
		if heading == phrase {
			return true
		}
		rest, found := strings.CutPrefix(heading, phrase+" ")
		if !found {
			continue
		}
		if allTail(strings.Fields(rest)) {
			return true
		}
	}
	return false
}

// allTail reports whether every word is one headingTail admits.
func allTail(words []string) bool {
	for _, w := range words {
		if !headingTail[w] {
			return false
		}
	}
	return true
}

// normalizeHeading lowercases a heading and reduces it to ASCII letters, digits and
// single spaces, so "Requirements:", "REQUIREMENTS", "Nice-to-have" and "What you'll
// need" all reach the vocabulary in the one spelling it is written in.
//
// Transliteration lets an accented heading be spelled once: without it "Előnyt jelent"
// would arrive as "el nyt jelent". Same fold as [normalize.Slug].
func normalizeHeading(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(unidecode.Unidecode(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		case r == '\'' || r == '’':
			// An apostrophe joins rather than separates ("what youll need"); both
			// spellings occur in HTML.
		default:
			space = true
		}
	}
	return b.String()
}

// listItems returns the plain text of a list's items. A nested list's items are part
// of their parent item's text rather than entries of their own — a posting that
// qualifies a requirement with a sub-list is stating one requirement, not two.
func listItems(list *xhtml.Node) []string {
	var out []string
	for c := list.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xhtml.ElementNode || c.DataAtom != atom.Li {
			continue
		}
		if text := textOf(c); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// textOf returns a node's plain text: descendant markup stripped, entities already
// decoded by the parser, and whitespace collapsed to single spaces.
//
// A block boundary contributes a space of its own, else "Go<ul><li>generics</li></ul>"
// reads "Gogenerics". Fields collapses the extras.
func textOf(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		breaks := n.Type == xhtml.ElementNode && breaksLine(n.DataAtom)
		if breaks {
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if breaks {
			b.WriteByte(' ')
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// breaksLine reports whether an element separates the text around it, the way a
// browser renders it: a block element, a list item, or an explicit break.
func breaksLine(a atom.Atom) bool {
	switch a {
	case atom.Ul, atom.Ol, atom.Li, atom.P, atom.Div, atom.Br, atom.Tr, atom.Td, atom.Th:
		return true
	default:
		return false
	}
}

// DeriveJSON is Derive encoded for the jsonb requirements_derived column. An empty list
// encodes as `[]` rather than nil, so the NOT NULL column never receives a null.
func DeriveJSON(descriptionHTML string) []byte {
	reqs := Derive(descriptionHTML)
	if len(reqs) == 0 {
		return []byte("[]")
	}
	encoded, err := json.Marshal(reqs)
	if err != nil {
		return []byte("[]")
	}
	return encoded
}
