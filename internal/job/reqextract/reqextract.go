// Package reqextract derives a posting's own stated requirements from its
// description markup, with no model call. It is the second producer of the
// enrichment contract's `requirements` field: the LLM reads the postings that state
// their requirements as prose, this reads the ones that state them as a list under a
// heading, and the two union rather than compete (measured on prod 2026-09-04: the
// model had reached 2.9% of open postings, a heading-and-list is present in 23%).
//
// The extraction is dictionary-gated in the same sense the facet dictionaries are:
// a heading outside the vocabulary yields nothing, and there is deliberately no
// fallback that infers which list in a posting is the requirements list. The most
// common list in a job posting after the requirements is the benefits list, and
// reading perks as requirements is worse than reading nothing.
//
// The bound is enrich.BoundRequirements — its own, not a second copy of the
// numbers — so the two producers of the field cannot drift apart.
package reqextract

import (
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/strelov1/freehire/internal/ai/enrich"
)

// maxHeadingRunes bounds how long a candidate heading may be. Only the inline
// elements (`p`, `strong`, `b`, `div`) need it: a posting that opens with "The
// requirements for this role are flexible…" is a sentence, not a section title, and
// length is what separates the two without a second vocabulary of non-headings.
// h1–h6 are structurally headings and are not length-checked.
const maxHeadingRunes = 60

// preferredHeadings and requiredHeadings are the controlled vocabulary the
// derivation is gated on, normalized the way normalizeHeading normalizes a node's
// text. A heading matches when its normalized text EQUALS a phrase or BEGINS with
// one, so "Requirements:" and "Requirements for this role" both match while "What we
// offer" matches nothing.
//
// preferredHeadings is consulted first and must stay first: "preferred
// qualifications" also begins with nothing in requiredHeadings, but "qualifications
// (preferred)" would be decided by whichever list is read first, and the optional
// reading is the safe one — overstating a nice-to-have as a hard requirement is the
// error a reader cannot detect.
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
}

// closingHeadings are the section titles that END a requirements section without
// starting one. They exist because structure alone cannot tell a heading from a
// lead-in: a posting written in a rich-text editor heads its sections with a bolded
// paragraph, which is the same shape `<p>You will need:</p>` takes. An h1–h6 outside
// the vocabulary closes a section on its tag alone; an inline element needs to be
// RECOGNIZED, or the spacers and lead-ins between a heading and its bullets would
// close it too.
//
// Everything here names the section a posting most often puts right after its
// requirements — which is the benefits list, the one thing that must never be read as
// a requirement. An unrecognized inline line still leaves the section open: this is a
// dictionary, so what it does not know, it does not act on.
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
}

var requiredHeadings = []string{
	"requirements",
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
		// x/net/html does not reject malformed markup, so this is unreachable in
		// practice; an unparseable description simply yields nothing rather than
		// failing a crawl.
		return nil
	}

	var found []enrich.Requirement
	// priority is the section currently open: the priority of the last recognized
	// heading, or "" when none is. Three things close a section, and between them they
	// are what stops a benefits list two elements down from being read as requirements:
	// taking its list, an unrecognized structural heading, and prose.
	priority := ""

	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch {
			// A container that holds the list is a wrapper, not a heading, however
			// short its text reads. Classifying it as one skipped its whole subtree,
			// which made whether a posting yielded anything depend on how long its
			// bullets happened to be.
			case wrapsAList(n):

			case isHeadingCandidate(n):
				priority = headingDecision(n, priority)
				return // the heading's own text is never an item

			case priority != "" && (n.DataAtom == atom.Ul || n.DataAtom == atom.Ol):
				for _, text := range listItems(n) {
					found = append(found, enrich.Requirement{Text: text, Priority: priority})
				}
				priority = "" // only the FIRST list after a heading is the section's
				return        // nested lists were consumed by listItems

			// Content, not a lead-in. A text block reaching this case has already
			// failed isHeadingCandidate, so it is one too long to be a title — prose.
			// Whatever list follows prose or a table is no longer the heading's, and
			// this is what keeps "Requirements" over a paragraph from claiming the
			// benefits list further down.
			case isTextBlock(n), n.DataAtom == atom.Table:
				priority = ""
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return enrich.BoundRequirements(found)
}

// isTextBlock reports whether an element is one of the four a posting uses for both
// its section titles and its paragraphs. Which of the two a given one IS depends on its
// length, not its tag — see isHeadingCandidate.
func isTextBlock(n *xhtml.Node) bool {
	switch n.DataAtom {
	case atom.P, atom.Strong, atom.B, atom.Div:
		return true
	default:
		return false
	}
}

// wrapsAList reports whether a text block that could otherwise pass for a heading is
// really a container holding the list. `<div><h3>Requirements</h3><ul>…</ul></div>` is
// one element whose whole text is short, and reading it as a heading discards the list
// inside it.
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
// an h1–h6, which is structurally one, or a text block short enough to be a line rather
// than a paragraph. What such a node then DOES to the open section is headingDecision's
// answer, not this one's.
//
// The length test appears only here. Its other half — "a text block too long to be a
// title is prose" — is not a second test but the walk reaching its later case, where a
// text block has already failed this one.
func isHeadingCandidate(n *xhtml.Node) bool {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return true
	default:
		return isTextBlock(n) && len([]rune(textOf(n))) <= maxHeadingRunes
	}
}

// headingDecision returns the section priority in force after this node, given the one
// in force before it. Three outcomes, and the difference between them is what decides
// whether a list two elements down is read as requirements:
//
//   - The text is in the requirements vocabulary: it OPENS a section at that priority.
//   - The text is in closingHeadings, or it is a structural heading (h1–h6) outside
//     every vocabulary: it CLOSES the open section — "Benefits" ends "Requirements".
//   - Anything else inline LEAVES the section as it was. This is the spacer `<p></p>`
//     and the lead-in `<p>You will need:</p>` that a rich-text editor puts between a
//     heading and its bullets; closing on those cost real postings their lists.
//
// The asymmetry between the last two is the whole reason closingHeadings exists. An
// h1–h6 announces itself as a heading by its tag, so an unrecognized one can safely
// close. An inline element does not: `<p>Benefits:</p>` and `<p>You will need:</p>`
// are the same shape, and only a vocabulary tells them apart.
//
// Prose never reaches here: a text block longer than maxHeadingRunes is not a
// candidate, and the walk closes the section on it in a later case instead.
func headingDecision(n *xhtml.Node, open string) string {
	heading := normalizeHeading(textOf(n))
	if matches(heading, preferredHeadings) {
		return "preferred"
	}
	if matches(heading, requiredHeadings) {
		return "required"
	}
	if matches(heading, closingHeadings) {
		return ""
	}
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return ""
	default:
		return open
	}
}

// headingTail is the closed set of words that may follow a vocabulary phrase without
// changing what the heading is: connectives, and the nouns a posting uses to name the
// same section twice ("Requirements & Qualifications", "Preferred competencies and
// qualifications"). A word outside this set means the line names something else.
//
// This set is what separates a section title from a sentence that opens with the same
// words. Both of these are real prod headings: "Required Qualifications & Skills"
// heads a requirements list, and "MUST HAVE MORNING/DAYTIME AVAILABILITY" heads a
// list of employee benefits. Only the tail tells them apart.
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
// Prefix matching alone is not enough. It admits "Requirements for this role" — which
// is wanted — but also "Must have morning availability", which is a sentence, and the
// list beneath such a line is as likely to be benefits as requirements. Requiring the
// remainder to be vocabulary is what keeps a statement from opening a section.
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

// normalizeHeading lowercases a heading and reduces it to letters, digits and single
// spaces, so "Requirements:", "REQUIREMENTS", "Nice-to-have" and "What you'll need"
// all reach the vocabulary in the one spelling it is written in.
func normalizeHeading(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		case r == '\'' || r == '’':
			// An apostrophe joins rather than separates: "what you'll need" becomes
			// "what youll need", one token, rather than "what you ll need". Both
			// spellings of the character, since a posting's HTML carries either.
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
// A block boundary contributes a space of its own. Markup is where the separation
// lives — "Go<ul><li>generics</li></ul>" carries no whitespace between the two words
// — so without this a qualified requirement reads "Gogenerics". The extra spaces are
// harmless: Fields collapses them.
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
