package reqextract

import (
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
)

// MaskPreferred returns the description with the WORDS inside recognized preferred-only
// sections blanked and everything else (markup, punctuation, layout) left in place, so
// the fact matchers in internal/job/jobfacts share Derive's section vocabulary.
//
// It blanks rather than deletes because those matchers read punctuation as structure:
// EnglishLevel binds a level word to an English keyword only when no "." or newline sits
// between them, so rejoining halves of a removed clause would drop the level from
// "English, B2 required".
//
// Unlike Derive, which closes a section on prose, a preferred SECTION is preferred
// throughout: only another heading closes it. A section never closed masks the rest of
// the description, which understates requirements (the detectable direction).
//
// A description with no preferred section comes back byte-identical.
func MaskPreferred(descriptionHTML string) string {
	if strings.TrimSpace(descriptionHTML) == "" {
		return descriptionHTML
	}
	doc, err := xhtml.Parse(strings.NewReader(descriptionHTML))
	if err != nil {
		// x/net/html does not reject malformed markup, so this is effectively unreachable.
		return descriptionHTML
	}

	masked := false
	preferred := false
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch {
			// A container holding the section's list is a wrapper, not a heading.
			case wrapsAList(n):

			case isHeadingCandidate(n):
				// Only a RECOGNIZED title (or h1–h6) decides anything; an unrecognized
				// short line is content, and falling through masks its words.
				next, recognized := headingDecision(n, openSection(preferred))
				if !recognized {
					break
				}
				preferred = next == "preferred"
				if !preferred {
					// A required or closing title's own words stay.
					return
				}
				// A preferred title is blanked with its section; left legible, the
				// caller's clause-level pass would find the marker and blank the
				// sentence around it, requirements included.
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

// restoreLiterals undoes the escaping the re-render ADDS: x/net/html escapes the literal
// apostrophe, quote and ">" of a description, and the matchers would then miss
// `bachelor&#39;s`. `&amp;` and `&lt;` are deliberately absent: the source already wrote
// those, so undoing them would change the text.
var restoreLiterals = strings.NewReplacer(
	"&#39;", "'",
	"&#34;", `"`,
	"&gt;", ">",
)

// openSection expresses the walk's boolean state in headingDecision's vocabulary.
func openSection(preferred bool) string {
	if preferred {
		return "preferred"
	}
	return ""
}

// BlankWords replaces every letter and digit with a space, leaving punctuation and
// whitespace alone. Unicode-aware.
//
// Exported because jobfacts also blanks optional clauses inside sentences, and both
// passes must blank identically (deleting would move sentence boundaries).
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
