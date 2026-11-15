package search

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// plainTextPolicy strips ALL HTML to bare text for embedding, unlike
// sources.descriptionPolicy, which keeps structural tags for rendering. Safe for
// concurrent use.
var plainTextPolicy = bluemonday.StrictPolicy()

// blockBoundary matches closing or self-closing block-level tags. It runs BEFORE
// plainTextPolicy because bluemonday drops tags without a separator, so
// "<p>A</p><p>B</p>" would become "AB".
//
// Limitation: only an EXPLICIT close matches. An implicitly closed <li>/<p> ("<li>One<li>Two")
// stays glued; fixing it needs a parsed tree (golang.org/x/net/html.Parse).
var blockBoundary = regexp.MustCompile(`(?i)</(h[1-6]|p|li|div|tr|td|th|table|blockquote|pre)\s*>|<(br|hr)\s*/?>`)

// collapseBlankLines folds runs of 2+ newlines (whitespace between allowed) so nested
// block elements do not each add a blockBoundary newline.
var collapseBlankLines = regexp.MustCompile(`\n[ \t]*\n(\n[ \t]*)*`)

// stripToPlainText renders description HTML into plain prose for embedding only. It keeps
// paragraph, list and table-row boundaries as line breaks for chunkText.
func stripToPlainText(rawHTML string) string {
	marked := blockBoundary.ReplaceAllString(rawHTML, "$0\n")
	// bluemonday re-escapes &, <, > when serializing, so "R&amp;D" would reach the model
	// as markup; unescape AFTER Sanitize.
	text := html.UnescapeString(plainTextPolicy.Sanitize(marked))
	text = collapseBlankLines.ReplaceAllString(text, "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
