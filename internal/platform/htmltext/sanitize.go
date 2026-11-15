// This file holds the description sanitizer: HTML processing with no knowledge of boards or
// postings, shared by packages outside ingest.
package htmltext

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// noBreakSpaces folds no-break spaces (bluemonday decodes &nbsp; to U+00A0) into regular
// ones: a description glued by them renders as one unbreakable, overflowing line.
var noBreakSpaces = strings.NewReplacer(
	" ", " ", // NO-BREAK SPACE
	" ", " ", // NARROW NO-BREAK SPACE
)

// descriptionPolicy sanitizes third-party job description HTML. It is an explicit prose
// allowlist, not UGCPolicy: no scripts, styles, forms or media (`<img>` would let a posting
// fetch a tracking pixel from every viewer rendered with `{@html}`).
//
// `a` is deliberately absent: bluemonday unwraps a disallowed element and keeps its text,
// so aggregator backlinks woven through sentences leave the prose whole.
var descriptionPolicy = newDescriptionPolicy()

func newDescriptionPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "br", "hr", "blockquote", "pre", "code", "div", "span",
		"ul", "ol", "li", "dl", "dt", "dd",
		"table", "thead", "tbody", "tr", "th", "td",
		"strong", "em", "b", "i", "u",
	)
	return p
}

// Sanitize returns s with active content, media, and links removed, safe to render directly.
//
// Self-labelled anchors are dropped BEFORE the policy runs: afterwards the unwrapped text can
// no longer be compared with its destination. The literal-newline repair runs first because
// it reads the surrounding markup and may emit a <br> the policy must vet.
func Sanitize(s string) string {
	// JSON-LD can decode a \u0000 escape into NUL. PostgreSQL text cannot
	// store it; remove it before parsing so it cannot split an HTML tag either.
	s = strings.ReplaceAll(s, "\x00", "")
	stripped, dropped := dropSelfLabelledAnchors(repairLiteralNewlines(s))
	out := noBreakSpaces.Replace(descriptionPolicy.Sanitize(stripped))
	if dropped {
		// Only a drop can empty a block, so a body with nothing stripped is never rewritten.
		out = dropEmptyBlocks(out)
	}
	// After wrapOrphanListItems: its re-render escapes quotes again.
	return restoreTextQuotes(wrapOrphanListItems(out))
}

// textQuotes undoes &#39; and &#34;, including the doubled &amp;#39; / &amp;#34; a feed
// stored one escape earlier. &amp; stays: a bare & is not valid text in the HTML
// this column stores. The contains check skips the copy Replace makes when
// nothing matches.
var textQuotes = strings.NewReplacer(
	"&amp;amp;#39;", "'",
	"&amp;amp;#34;", `"`,
	"&amp;#39;", "'",
	"&amp;#34;", `"`,
	"&#39;", "'",
	"&#34;", `"`,
)

func restoreTextQuotes(s string) string {
	if strings.Contains(s, "&#39;") || strings.Contains(s, "&#34;") ||
		strings.Contains(s, "amp;#39;") || strings.Contains(s, "amp;#34;") {
		return textQuotes.Replace(s)
	}
	return s
}

// literalNewlineMarker is the backslash plus "n" a double-escaping feed leaves in place of a
// line break.
const literalNewlineMarker = `\n`

var (
	// tagAdjacentLiteralNewline matches an escape against a tag on either side (whitespace
	// allowed between): source indentation, i.e. whitespace only. One alternation, so an escape
	// between two tags ("</p>\n<p>") is rewritten once.
	tagAdjacentLiteralNewline = regexp.MustCompile(`(>\s*)\\n|\\n(\s*<)`)
	// looseLiteralNewline matches the escapes left over: prose on both sides, the only line break.
	looseLiteralNewline = regexp.MustCompile(`\\n`)
)

// repairLiteralNewlines restores line breaks that a double-escaping feed turned into a visible
// literal "\n". What an escape meant depends on where it sits:
//
//   - Against a tag it is indentation between blocks: it becomes a real newline. A <br> there
//     would add blank lines, and inside a <ul> a browser hoists it out of the list.
//   - Between two sentences it is the posting's only break: it becomes a <br>.
//
// shortcut: a "\n" deliberately shown as a code escape is rewritten too; exempt <code>/<pre>
// via the tree walk in wrapOrphanListItems if a real case appears.
func repairLiteralNewlines(s string) string {
	if !strings.Contains(s, literalNewlineMarker) {
		return s
	}
	s = tagAdjacentLiteralNewline.ReplaceAllString(s, "${1}\n${2}")
	return looseLiteralNewline.ReplaceAllString(s, "<br>")
}

// wrapOrphanListItems wraps each maximal run of <li> siblings lacking a <ul>/<ol> parent in a
// synthetic <ul>; the policy allows <li> anywhere, so a bare one would render as a stray bullet.
// Finding orphans needs the parsed tree, not a regex. Returns s untouched when nothing needs
// wrapping, since re-serializing renormalizes markup (<br> becomes <br/>).
func wrapOrphanListItems(s string) string {
	if !strings.Contains(s, "<li") {
		return s
	}
	nodes, err := xhtml.ParseFragment(strings.NewReader(s), &xhtml.Node{
		Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div,
	})
	if err != nil {
		return s // malformed beyond repair — leave sanitizeHTML's output as-is
	}

	root := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	if !wrapOrphanListItemsIn(root) {
		return s
	}

	var buf strings.Builder
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		_ = xhtml.Render(&buf, c)
	}
	return buf.String()
}

// wrapOrphanListItemsIn recurses depth-first, then splices each run of consecutive
// <li>/whitespace-text siblings under n into a new <ul>, unless n is already a <ul>/<ol>.
// Whitespace between items joins the run so spacing stays inside the wrapper. Reports whether
// it wrapped anything, which gates re-serialization.
func wrapOrphanListItemsIn(n *xhtml.Node) bool {
	changed := false
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		// The call only splices c's own children, so c.NextSibling stays valid.
		if c.Type == xhtml.ElementNode && wrapOrphanListItemsIn(c) {
			changed = true
		}
	}

	if n.DataAtom == atom.Ul || n.DataAtom == atom.Ol {
		return changed
	}

	var run []*xhtml.Node
	hasLi := false
	flush := func() {
		if !hasLi {
			run = nil
			return
		}
		ul := &xhtml.Node{Type: xhtml.ElementNode, Data: "ul", DataAtom: atom.Ul}
		n.InsertBefore(ul, run[0])
		for _, item := range run {
			n.RemoveChild(item)
			ul.AppendChild(item)
		}
		run, hasLi = nil, false
		changed = true
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == xhtml.ElementNode && c.DataAtom == atom.Li:
			run = append(run, c)
			hasLi = true
		case c.Type == xhtml.TextNode && strings.TrimSpace(c.Data) == "":
			run = append(run, c)
		default:
			flush()
		}
	}
	flush()
	return changed
}

var (
	// anchorTag matches one anchor: its attribute run and its inner HTML. Anchors cannot
	// nest in HTML, so the non-greedy body cannot swallow a following link.
	anchorTag = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	// anchorHref reads the href value out of an attribute run, quoted either way or bare.
	anchorHref = regexp.MustCompile(`(?is)\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	// innerTag strips the markup nested inside an anchor, leaving its visible text.
	innerTag = regexp.MustCompile(`<[^>]*>`)
	// addressLikeText matches visible text that reads as a web address rather than prose:
	// a scheme, a "www." host, or a dotted host with an optional path. It is only half the
	// test — "Node.js" matches this too — and is paired with the destination comparison.
	addressLikeText = regexp.MustCompile(`(?i)^(?:https?://|www\.)\S+$|^[\w-]+(?:\.[\w-]+)+(?:/\S*)?$`)
	// emptyBlock matches a block element left with nothing in it. The pairs are spelled out
	// because RE2 has no backreference; a heading may close on a different level, which is
	// malformed input either way and empty in both readings.
	emptyBlock = regexp.MustCompile(`(?is)<p>\s*</p>|<li>\s*</li>|<div>\s*</div>|<blockquote>\s*</blockquote>|<td>\s*</td>|<h[1-6]>\s*</h[1-6]>`)
)

// dropSelfLabelledAnchors removes anchors whose text merely repeats their destination
// ("<a href=x.co/1>https://x.co/1</a>"), which would leave a dead address in the prose.
// The test is two-sided: the text must LOOK like an address and MATCH the href ("Node.js"
// reads as a host; a one-word label can prefix its URL by chance). Reports whether anything
// was dropped, which gates the empty-block sweep.
func dropSelfLabelledAnchors(s string) (string, bool) {
	if !strings.Contains(s, "<a") && !strings.Contains(s, "<A") {
		return s, false
	}
	dropped := false
	out := anchorTag.ReplaceAllStringFunc(s, func(tag string) string {
		m := anchorTag.FindStringSubmatch(tag)
		if m == nil || !isSelfLabelled(m[1], m[2]) {
			return tag
		}
		dropped = true
		return ""
	})
	return out, dropped
}

// isSelfLabelled reports whether an anchor's text says nothing beyond its own address.
// Only http(s) destinations qualify: a mailto's text is a contact address the reader
// needs, and it survives the unwrap as prose.
func isSelfLabelled(attrs, inner string) bool {
	text := strings.TrimSpace(html.UnescapeString(innerTag.ReplaceAllString(inner, "")))
	if !addressLikeText.MatchString(text) {
		return false
	}
	m := anchorHref.FindStringSubmatch(attrs)
	if m == nil {
		return false
	}
	href := strings.ToLower(strings.TrimSpace(m[1] + m[2] + m[3]))
	if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
		return false
	}
	label, target := normalizeAddress(text), normalizeAddress(href)
	return strings.HasPrefix(label, target) || strings.HasPrefix(target, label)
}

// normalizeAddress strips scheme, "www." and trailing slash so a label and its href compare equal.
func normalizeAddress(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	return strings.TrimSuffix(s, "/")
}

// dropEmptyBlocks removes blocks a dropped anchor left empty, repeating until stable so nested
// ones ("<div><p></p></div>") go too.
func dropEmptyBlocks(s string) string {
	for {
		next := emptyBlock.ReplaceAllString(s, "")
		if next == s {
			return s
		}
		s = next
	}
}
