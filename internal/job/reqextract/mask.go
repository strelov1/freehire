package reqextract

import (
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
)

// MaskPreferred returns the description with the WORDS inside recognized
// preferred-only sections blanked and everything else — markup, punctuation, layout
// — left in place. It exists so the deterministic fact matchers in
// internal/job/jobfacts read the same section vocabulary Derive does instead of each
// guessing on its own where a nice-to-have section ends.
//
// Blanking rather than deleting is the whole point. Those matchers read punctuation
// as structure: EnglishLevel binds a level word to an English keyword only when no
// "." or newline sits between them, so a masker that removed a clause and rejoined
// the halves would introduce a boundary that was never in the posting and silently
// drop the level from "English, B2 required". Replacing letters and digits with
// spaces leaves every boundary exactly where the posting put it.
//
// It differs from Derive in one deliberate way: Derive closes a section on prose,
// because it wants only the list under a heading, while a preferred SECTION is
// preferred throughout — the issue this fixes was reported against a posting whose
// nice-to-have section is ordinary paragraphs. Only another heading closes a masked
// section here. A preferred section a posting never closes therefore masks the rest
// of the description, which understates requirements rather than overstating them —
// the direction a reader can detect.
//
// A description with no preferred section comes back byte-identical: the re-render is
// skipped entirely, so nothing downstream can shift for a posting this has no opinion
// about.
func MaskPreferred(descriptionHTML string) string {
	if strings.TrimSpace(descriptionHTML) == "" {
		return descriptionHTML
	}
	doc, err := xhtml.Parse(strings.NewReader(descriptionHTML))
	if err != nil {
		// x/net/html does not reject malformed markup, so this is unreachable in
		// practice; an unparseable description is simply left as it is.
		return descriptionHTML
	}

	masked := false
	preferred := false
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch {
			// A container holding the section's list is a wrapper, not a heading —
			// the same trap Derive's walk names.
			case wrapsAList(n):

			case isHeadingCandidate(n):
				// Only a RECOGNIZED title (or a structural h1–h6) decides anything. An
				// unrecognized short line — the lead-in `<p>You will need:</p>`, or a
				// one-sentence paragraph inside a nice-to-have section — is content,
				// and falling through is what masks the words it carries.
				next, recognized := headingDecision(n, openSection(preferred))
				if !recognized {
					break
				}
				preferred = next == "preferred"
				if !preferred {
					// A required or closing title is outside the section it ends, so its
					// own words stay.
					return
				}
				// A preferred title's words are blanked with the section it opens, and
				// the walk descends to do it. Leaving "Nice to have" legible would put
				// the marker phrase back in the text for the caller's second, clause-level
				// pass to find — which then blanks the sentence around it, requirements
				// and all.
			}
		}
		if n.Type == xhtml.TextNode && preferred {
			if blanked := BlankWords(n.Data); blanked != n.Data {
				n.Data = blanked
				masked = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if !masked {
		return descriptionHTML
	}
	var b strings.Builder
	if err := xhtml.Render(&b, doc); err != nil {
		return descriptionHTML
	}
	return restoreLiterals.Replace(b.String())
}

// restoreLiterals undoes the escaping the re-render ADDS. A description carries an
// apostrophe, a quote and often a bare ">" as literal characters; x/net/html escapes
// all three on the way out, and the matchers reading this text see `bachelor&#39;s`,
// which their vocabulary does not contain. That broke exactly the postings this masker
// exists for — only a posting WITH a preferred section is re-rendered at all — and
// every unit test stayed green, because none of them spelled a degree with an
// apostrophe inside a document that had a preferred section.
//
// `&amp;` and `&lt;` are deliberately not here: those are escapes the SOURCE already
// wrote, which the parser decoded and the renderer restored. Undoing them would not be
// a round trip, it would be a change.
var restoreLiterals = strings.NewReplacer(
	"&#39;", "'",
	"&#34;", `"`,
	"&gt;", ">",
)

// openSection expresses the walk's boolean state in headingDecision's vocabulary, so
// the two callers share one decision function rather than one of them re-deciding
// what a heading means.
func openSection(preferred bool) string {
	if preferred {
		return "preferred"
	}
	return ""
}

// BlankWords replaces every letter and digit with a space, leaving punctuation and
// whitespace alone. Unicode-aware, because a preferred section is as often Hungarian,
// Polish or Russian as English.
//
// It is exported because [MaskPreferred] is only half of what internal/job/jobfacts
// needs — a posting states optionality as a section OR as a clause inside a sentence —
// and the two passes have to blank identically. Blanking is what makes either pass
// safe: the matchers downstream read punctuation as structure, so a pass that DELETED
// its span would move a sentence boundary and change what the other pass sees.
func BlankWords(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
