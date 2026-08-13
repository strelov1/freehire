package sources

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// noBreakSpaces normalizes the no-break space characters that some ATS boards use
// in place of regular spaces (commonly the &nbsp; entity, which bluemonday decodes
// to the raw U+00A0 rune). Left as-is, a description whose every word is glued by
// U+00A0 renders as one unbreakable line that overflows the page horizontally, so
// we fold them to a regular space, restoring word-boundary wrapping. Compiled once
// and safe for concurrent use, like descriptionPolicy.
var noBreakSpaces = strings.NewReplacer(
	" ", " ", // NO-BREAK SPACE
	" ", " ", // NARROW NO-BREAK SPACE
)

// descriptionPolicy sanitizes source-provided job description HTML. It is compiled
// once and reused: bluemonday policies are safe for concurrent use.
//
// It is an explicit prose allowlist rather than bluemonday's UGCPolicy: descriptions
// come from third-party ATS boards, so we keep only the structural formatting we
// render (headings, paragraphs, lists, tables, emphasis) and drop everything that
// triggers requests or execution — scripts, styles, forms, and crucially media
// (`<img>`), which would otherwise let a posting fetch a tracking pixel against every
// viewer when rendered with `{@html}`.
//
// `a` is deliberately absent: a description is prose, not a set of exits. bluemonday
// unwraps an element it does not allow rather than deleting its content, so an anchor
// leaves its text — and any emphasis inside it — behind. That is what we want, because
// aggregators weave backlinks through ordinary sentences and deleting the text with the
// tag would punch holes in the prose.
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

// sanitizeHTML returns s with active content, media, and links removed, leaving HTML that
// is safe to render directly in a browser. Adapters call it on their assembled description
// HTML before yielding a job, so the catalogue stores only sanitized markup.
//
// Self-labelled anchors are dropped BEFORE the policy runs, because that is the only point
// at which an anchor's text can still be compared against its own destination — the policy
// unwraps the tag and the two become indistinguishable from prose.
func sanitizeHTML(s string) string {
	stripped, dropped := dropSelfLabelledAnchors(s)
	out := noBreakSpaces.Replace(descriptionPolicy.Sanitize(stripped))
	if dropped {
		// Only a drop can empty a block, so a body with nothing stripped is never rewritten.
		out = dropEmptyBlocks(out)
	}
	return wrapOrphanListItems(out)
}

// wrapOrphanListItems wraps each maximal run of <li> siblings that lacks a <ul>/<ol>
// parent in a synthetic <ul>. Some boards' source HTML drops the list wrapper partway
// through a description (seen live: a posting whose first two bullet groups are
// properly wrapped and a third is bare <li> siblings) — descriptionPolicy allows <li>
// as a structural element but, being a sanitizer and not a tree validator, does not
// require it to sit inside a list, so the malformed shape survives unchanged and
// renders as a stray, un-announced bullet.
//
// Detecting "orphan" correctly needs real tree structure, not a regex — a properly
// nested <li> inside <table>/<div> wrappers is indistinguishable from a bare one by
// pattern alone. Returns s completely untouched, not merely equivalent, whenever
// there is nothing to wrap (no <li> at all, or every one already sits in a list):
// re-serializing a parsed tree can renormalize spelling bluemonday itself chose
// (e.g. a void element written <br> comes back <br/> — equally valid HTML, but a
// needless rewrite of the vast majority of descriptions that have no orphan at all).
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

// wrapOrphanListItemsIn recurses depth-first (so a nested orphan run inside a <div> or
// <td> is fixed before its ancestor is considered), then splices every maximal run of
// consecutive <li>/whitespace-text siblings under n into a new <ul> — unless n is
// itself already a <ul>/<ol>, in which case its <li> children are already valid and
// it is left untouched. Whitespace-only text nodes between <li> siblings (the newline
// indentation typical of hand-written HTML) are pulled into the run too, so the
// original spacing between items is preserved inside the new wrapper instead of being
// severed from the list it separates. Reports whether it wrapped anything, which
// gates whether the caller re-serializes at all.
func wrapOrphanListItemsIn(n *xhtml.Node) bool {
	changed := false
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		// The recursive call only splices c's own children, never c's siblings,
		// so c.NextSibling stays valid across the call — unlike the second loop
		// below, which does reorder n's children and so must snapshot ahead.
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

// dropSelfLabelledAnchors removes anchors whose visible text merely repeats their own
// destination ("<a href=x.co/1>https://x.co/1</a>"). Unwrapped, such an anchor leaves a
// dead address sitting in the prose — it carries no information the link itself did not,
// and unlike a worded link there is no sentence to keep whole. Every other anchor is left
// for the policy to unwrap, text intact.
//
// The test is deliberately two-sided: the text must LOOK like an address and must MATCH
// the href. Either half alone misfires — "Node.js" reads as a host, and a one-word label
// can be a prefix of its own URL by chance.
//
// It reports whether anything was dropped, which is what gates the empty-block sweep.
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

// normalizeAddress reduces a web address to the part that carries its identity, so a label
// and its href compare equal across the cosmetic differences between how a board writes the
// two (scheme present or not, "www." present or not, trailing slash or not).
func normalizeAddress(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	return strings.TrimSuffix(s, "/")
}

// dropEmptyBlocks removes block elements a dropped anchor left empty, which would otherwise
// render as a stray gap in the description. It repeats until the body stops changing, so a
// block whose only child was itself emptied ("<div><p></p></div>") goes too; each pass only
// shortens the string, so the loop terminates.
func dropEmptyBlocks(s string) string {
	for {
		next := emptyBlock.ReplaceAllString(s, "")
		if next == s {
			return s
		}
		s = next
	}
}

// SanitizeHTML is the exported description sanitizer, for sibling packages that build
// sources.Job values outside this package (e.g. internal/linksource).
func SanitizeHTML(s string) string { return sanitizeHTML(s) }

// encodedTagOpener and liveTagOpener count tag openers in their entity-encoded ("&lt;div",
// "&lt;/div") and live ("<div", "</div") forms. Both require a letter after the bracket, so a
// lone "<" or "&lt;" standing in for a less-than sign in prose counts as neither.
var (
	encodedTagOpener = regexp.MustCompile(`&lt;/?[a-zA-Z]`)
	liveTagOpener    = regexp.MustCompile(`</?[a-zA-Z]`)
)

// unescapeEncodedHTML undoes one layer of HTML entity-encoding when s carries its markup as
// text ("&lt;p&gt;Role&lt;/p&gt;") rather than as live HTML. Some feeds encode a posting's
// body before serving it, and sanitizeHTML cannot recover that on its own: bluemonday reads
// "&lt;p&gt;" as a text node and re-encodes it on output, so the tags reach the catalogue as
// literals the reader sees instead of the structure they describe.
//
// The decision is by weight, not by presence: encoding is undone only when encoded tag
// openers outnumber live ones. Decoding unconditionally would corrupt the healthy majority of
// such a feed — a posting that deliberately shows markup as an example would have that example
// turned into real tags and then stripped, silently losing the content — whereas a wholly
// encoded body stays dominated by encoded openers even when the feed wraps it in live HTML of
// its own (arbeitnow appends a promo footer).
// UnescapeEncodedHTML is the exported form of the entity-encoding repair, for the
// description backfill worker that re-runs the adapter pipeline over stored rows.
func UnescapeEncodedHTML(s string) string { return unescapeEncodedHTML(s) }

func unescapeEncodedHTML(s string) string {
	// Fast path: no encoded markup to weigh.
	if !strings.Contains(s, "&lt;") {
		return s
	}
	if len(encodedTagOpener.FindAllStringIndex(s, -1)) <= len(liveTagOpener.FindAllStringIndex(s, -1)) {
		return s
	}
	return html.UnescapeString(s)
}

// IsRemote is the exported form of the shared location-based remote heuristic, so sibling
// packages flag remote jobs consistently with the ATS adapters.
func IsRemote(location string) bool { return isRemote(location) }

// LenientPercentUnescape percent-decodes every valid "%XX" (two hex digits) sequence and
// passes any stray "%" through literally. It exists because Go's url.PathUnescape is strict:
// a single "%" not followed by two hex digits (common in Word-pasted ATS HTML, e.g. the CSS
// "line-height:115%") makes it reject the ENTIRE string, so callers that fell back to the
// raw value stored a still-fully-encoded description. Like PathUnescape it leaves "+" intact
// so tokens like "C++" survive. Decoding is byte-wise (percent-encoding is defined on bytes),
// so multi-byte UTF-8 sequences reassemble correctly.
func LenientPercentUnescape(s string) string {
	// Fast path: nothing to decode.
	if !strings.ContainsRune(s, '%') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b.WriteByte(unhex(s[i+1])<<4 | unhex(s[i+2]))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}
