package search

import (
	"strings"
	"testing"
)

// stripToPlainText must produce tag-free prose for the embedding path, and must not
// glue adjacent block elements together: bluemonday.StrictPolicy() alone drops tags
// without inserting any separator, so "<p>A</p><p>B</p>" strips to "AB" — silently
// corrupting the text the embedding model sees.
func TestStripToPlainText(t *testing.T) {
	html := "<h2>Requirements</h2>" +
		"<ul><li>Go</li><li>Postgres</li></ul>" +
		"<p>Nice to have: <strong>Kubernetes</strong> and <em>Docker</em>.</p>" +
		"<p>Second paragraph here.</p>" +
		"<table><tr><td>Location</td><td>Remote</td></tr></table>"

	got := stripToPlainText(html)

	if strings.ContainsAny(got, "<>") {
		t.Fatalf("stripToPlainText left tag soup: %q", got)
	}
	for _, want := range []string{
		"Requirements", "Go", "Postgres",
		"Nice to have: Kubernetes and Docker.",
		"Second paragraph here.",
		"Location", "Remote",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stripToPlainText(%q) = %q, missing %q", html, got, want)
		}
	}
	for _, glued := range []string{"RequirementsGo", "GoPostgres", "Docker.Second", "LocationRemote"} {
		if strings.Contains(got, glued) {
			t.Fatalf("stripToPlainText(%q) = %q, glued block boundary %q", html, got, glued)
		}
	}
}

// bluemonday's Sanitize re-escapes &, <, > when it re-serializes text nodes (all other
// entities decode fine) — left alone, "R&amp;D" would reach the embedding model as
// literal markup, the exact "tag soup mixed into prose" problem this function exists
// to eliminate (proposal.md motivation #1), just at the entity level instead of the
// tag level.
func TestStripToPlainTextDecodesEntities(t *testing.T) {
	html := "<p>R&amp;D and Sales &amp; Marketing, 5+ yrs &gt; junior, &rsquo;24 cohort</p>"
	want := "R&D and Sales & Marketing, 5+ yrs > junior, ’24 cohort"
	if got := stripToPlainText(html); got != want {
		t.Fatalf("stripToPlainText(%q) = %q, want %q", html, got, want)
	}
}
