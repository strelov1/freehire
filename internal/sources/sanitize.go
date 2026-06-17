package sources

import (
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

// IsRemote is the exported form of the shared location-based remote heuristic, so sibling
// packages flag remote jobs consistently with the ATS adapters.
func IsRemote(location string) bool { return isRemote(location) }
