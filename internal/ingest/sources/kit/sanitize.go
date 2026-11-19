package kit

import (
	"html"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// SanitizeHTML is the package-local name for the description sanitizer in
// internal/platform/htmltext, which knows nothing about boards and is shared outside ingest.
func SanitizeHTML(s string) string { return htmltext.Sanitize(s) }

// EncodedTagOpener and LiveTagOpener count tag openers in their entity-encoded ("&lt;div",
// "&lt;/div") and live ("<div", "</div") forms. Both require a letter after the bracket, so a
// lone "<" or "&lt;" standing in for a less-than sign in prose counts as neither.
var (
	EncodedTagOpener = regexp.MustCompile(`&lt;/?[a-zA-Z]`)
	LiveTagOpener    = regexp.MustCompile(`</?[a-zA-Z]`)
)

// UnescapeEncodedHTML undoes one layer of HTML entity-encoding when s carries its markup as
// text ("&lt;p&gt;Role&lt;/p&gt;") rather than as live HTML. Some feeds encode a posting's
// body before serving it, and SanitizeHTML cannot recover that on its own: bluemonday reads
// "&lt;p&gt;" as a text node and re-encodes it on output, so the tags reach the catalogue as
// literals the reader sees instead of the structure they describe.
//
// The decision is by weight, not presence: encoding is undone only when encoded tag openers
// outnumber live ones. Decoding unconditionally would turn a posting's deliberate markup
// example into real tags that are then stripped, losing content, whereas a wholly encoded body
// stays dominated by encoded openers even when the feed wraps it in live HTML (arbeitnow
// appends a promo footer).
func UnescapeEncodedHTML(s string) string {
	// Fast path: no encoded markup to weigh.
	if !strings.Contains(s, "&lt;") {
		return s
	}
	if len(EncodedTagOpener.FindAllStringIndex(s, -1)) <= len(LiveTagOpener.FindAllStringIndex(s, -1)) {
		return s
	}
	return html.UnescapeString(s)
}

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
