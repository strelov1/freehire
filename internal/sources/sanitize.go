package sources

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
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
// render (headings, paragraphs, lists, tables, emphasis, links) and drop everything
// that triggers requests or execution — scripts, styles, forms, and crucially media
// (`<img>`), which would otherwise let a posting fetch a tracking pixel against every
// viewer when rendered with `{@html}`. Links are kept but marked nofollow so untrusted
// postings cannot pass link authority.
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
	p.AllowAttrs("href").OnElements("a")
	p.AllowStandardURLs()          // http/https/mailto schemes only
	p.RequireNoFollowOnLinks(true) // defang untrusted outbound links
	return p
}

// sanitizeHTML returns s with active content and media removed, leaving HTML that is
// safe to render directly in a browser. Adapters call it on their assembled description
// HTML before yielding a job, so the catalogue stores only sanitized markup.
func sanitizeHTML(s string) string {
	return noBreakSpaces.Replace(descriptionPolicy.Sanitize(s))
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
