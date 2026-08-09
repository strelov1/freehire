package search

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// plainTextPolicy strips ALL HTML down to bare text for the embedding path. Unlike
// sources.descriptionPolicy (a render-safety allowlist that deliberately KEEPS
// structural tags for `{@html}`), the embedding model needs prose, not markup — this
// is bluemonday's own built-in "remove every tag" policy, not a new render allowlist.
// Compiled once and reused: bluemonday policies are safe for concurrent use.
var plainTextPolicy = bluemonday.StrictPolicy()

// blockBoundary matches the closing/self-closing tags of block-level elements
// (headings, paragraphs, list items, table cells/rows, line breaks, dividers). It
// runs BEFORE plainTextPolicy strips tags: bluemonday drops tags without inserting
// any separator, so "<p>A</p><p>B</p>" would otherwise sanitize to "AB" — gluing
// unrelated sentences together and corrupting the text the embedding model sees.
//
// Known limitation: this only matches an EXPLICIT close. HTML5 lets <li>/<p> close
// implicitly (e.g. "<li>One<li>Two" needs no </li>), and bluemonday's tokenizer-based
// sanitizer — like jobs.description's own ingest-time sanitizer, sources.descriptionPolicy
// — does not rebuild a normalized DOM, so an implicitly-closed element from messy
// board HTML reaches here still unclosed and still glues. Fixing that would mean
// walking a parsed tree (golang.org/x/net/html.Parse, which does apply HTML5's
// implicit-close rules) instead of pattern-matching tokens — deferred until it's
// shown to matter for real description text.
var blockBoundary = regexp.MustCompile(`(?i)</(h[1-6]|p|li|div|tr|td|th|table|blockquote|pre)\s*>|<(br|hr)\s*/?>`)

// collapseBlankLines folds runs of 2+ newlines (with only whitespace between them)
// down to a single blank line, so nested block elements (e.g. a <li> inside a <ul>
// inside a <div>) don't each contribute their own blockBoundary newline and leave a
// wall of empty lines in the plain text.
var collapseBlankLines = regexp.MustCompile(`\n[ \t]*\n(\n[ \t]*)*`)

// stripToPlainText renders job description HTML into plain prose for the embedding
// path only — the rendered description served to the site is untouched. It preserves
// paragraph/list/table-row boundaries as line breaks so the chunker (chunkText) can
// split on them, rather than collapsing the whole description into one run-on line.
func stripToPlainText(rawHTML string) string {
	marked := blockBoundary.ReplaceAllString(rawHTML, "$0\n")
	// bluemonday re-escapes &, <, > when it re-serializes text nodes (every other
	// entity decodes fine during Sanitize) — left alone, "R&amp;D" would reach the
	// embedding model as literal markup, so UnescapeString runs AFTER Sanitize to
	// clean up what Sanitize itself re-introduced.
	text := html.UnescapeString(plainTextPolicy.Sanitize(marked))
	text = collapseBlankLines.ReplaceAllString(text, "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
