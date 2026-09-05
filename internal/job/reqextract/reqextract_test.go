package reqextract

import (
	"fmt"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/enrich"
)

// req is the shorthand the table below reads in: "required" and "preferred" are the
// only two priorities the enrichment contract admits.
func req(text string) enrich.Requirement {
	return enrich.Requirement{Text: text, Priority: "required"}
}

func pref(text string) enrich.Requirement {
	return enrich.Requirement{Text: text, Priority: "preferred"}
}

func TestDerive(t *testing.T) {
	tests := []struct {
		name string
		html string
		want []enrich.Requirement
	}{
		{
			name: "a requirements heading followed by a list yields its items",
			html: `<p>We are hiring.</p>
			       <h3>Requirements</h3>
			       <ul><li>5+ years of Go</li><li>Postgres</li></ul>`,
			want: []enrich.Requirement{req("5+ years of Go"), req("Postgres")},
		},
		{
			name: "an optionality heading yields preferred entries",
			html: `<h3>Nice to have</h3><ul><li>Kubernetes</li></ul>`,
			want: []enrich.Requirement{pref("Kubernetes")},
		},
		{
			name: "both sections yield both priorities, in document order",
			html: `<h2>Requirements</h2><ul><li>Go</li></ul>
			       <h2>Nice to have</h2><ul><li>Rust</li></ul>`,
			want: []enrich.Requirement{req("Go"), pref("Rust")},
		},
		{
			name: "a benefits list is not read as requirements",
			html: `<h3>What we offer</h3><ul><li>Free lunch</li><li>25 days holiday</li></ul>`,
			want: nil,
		},
		{
			name: "a matching heading with prose after it yields nothing",
			html: `<h3>Requirements</h3><p>You should be a self-starter who loves shipping.</p>`,
			want: nil,
		},
		{
			name: "a description with no markup yields nothing",
			html: `We need someone with five years of Go and a love of Postgres.`,
			want: nil,
		},
		{
			name: "only the first list after a heading is taken",
			html: `<h3>Requirements</h3><ul><li>Go</li></ul>
			       <ul><li>Free lunch</li></ul>`,
			want: []enrich.Requirement{req("Go")},
		},
		{
			name: "a non-matching heading closes the section",
			html: `<h3>Requirements</h3>
			       <h3>Benefits</h3>
			       <ul><li>Free lunch</li></ul>`,
			want: nil,
		},
		{
			name: "an ordered list counts the same as an unordered one",
			html: `<h3>Qualifications</h3><ol><li>A degree</li></ol>`,
			want: []enrich.Requirement{req("A degree")},
		},
		{
			name: "a bolded paragraph acts as a heading",
			html: `<p><strong>Requirements:</strong></p><ul><li>Go</li></ul>`,
			want: []enrich.Requirement{req("Go")},
		},
		{
			name: "a paragraph merely mentioning requirements is not a heading",
			html: `<p>The requirements for this role are flexible and we are happy to
			       discuss them with you at any point during the process.</p>
			       <ul><li>Free lunch</li></ul>`,
			want: nil,
		},
		{
			name: "item markup is reduced to plain text",
			html: `<h3>Requirements</h3><ul><li><strong>Go</strong> &mdash; 5 years</li></ul>`,
			want: []enrich.Requirement{req("Go — 5 years")},
		},
		{
			name: "whitespace inside an item is collapsed",
			html: "<h3>Requirements</h3><ul><li>\n  Go   and\tPostgres\n</li></ul>",
			want: []enrich.Requirement{req("Go and Postgres")},
		},
		{
			name: "an empty item is dropped rather than stored blank",
			html: `<h3>Requirements</h3><ul><li>  </li><li>Go</li></ul>`,
			want: []enrich.Requirement{req("Go")},
		},
		{
			name: "a nested list's items are not double-counted",
			html: `<h3>Requirements</h3><ul><li>Go<ul><li>generics</li></ul></li></ul>`,
			want: []enrich.Requirement{req("Go generics")},
		},
		{
			name: "an empty description yields nothing",
			html: ``,
			want: nil,
		},
		// The three below are real prod descriptions (2026-09-04). Each is a way a
		// vocabulary phrase appears inside a line that is not a section title, and
		// each one swept a benefits or scheduling list into the requirements before
		// the tail rule existed.
		{
			name: "a sentence beginning with a vocabulary phrase is not a heading",
			html: `<p><strong>MUST HAVE MORNING/DAYTIME AVAILABILITY</strong></p>
			       <ul><li>Competitive hourly wages</li><li>Health benefits</li></ul>`,
			want: nil,
		},
		{
			name: "a vocabulary phrase qualifying an unrelated noun is not a heading",
			html: `<p><strong>Preferred Hours:</strong></p><ul><li>Minimum 16 hours per week</li></ul>`,
			want: nil,
		},
		{
			name: "a vocabulary phrase qualified by another one is still a heading",
			html: `<h3>Preferred competencies and qualifications</h3><ul><li>A degree</li></ul>`,
			want: []enrich.Requirement{pref("A degree")},
		},
		{
			name: "two vocabulary nouns joined by an ampersand are still a heading",
			html: `<h3>Requirements &amp; Qualifications</h3><ul><li>Go</li></ul>`,
			want: []enrich.Requirement{req("Go")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(tt.html)
			if !equal(got, tt.want) {
				t.Errorf("Derive() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The two producers of enrichment.requirements must obey one ceiling, so the bound
// is enrich's own rather than a second copy of the numbers.
func TestDeriveObeysTheEnrichmentBounds(t *testing.T) {
	t.Run("an over-long list is truncated to the enrichment maximum", func(t *testing.T) {
		var items strings.Builder
		for i := range 200 {
			fmt.Fprintf(&items, "<li>skill %d</li>", i)
		}
		got := Derive("<h3>Requirements</h3><ul>" + items.String() + "</ul>")

		want := len(enrich.BoundRequirements(make([]enrich.Requirement, 200)))
		if want != 0 {
			t.Fatalf("bound of 200 blank entries = %d, want 0 — fix the probe", want)
		}
		bounded := enrich.BoundRequirements(got)
		if len(got) != len(bounded) {
			t.Errorf("Derive() returned %d entries, but the enrichment bound keeps %d", len(got), len(bounded))
		}
	})

	t.Run("an over-long entry is clipped to the enrichment maximum", func(t *testing.T) {
		long := strings.Repeat("x", 5000)
		got := Derive("<h3>Requirements</h3><ul><li>" + long + "</li></ul>")

		if len(got) != 1 {
			t.Fatalf("Derive() = %v, want one entry", got)
		}
		bounded := enrich.BoundRequirements([]enrich.Requirement{{Text: long, Priority: "required"}})
		if got[0].Text != bounded[0].Text {
			t.Errorf("clipped text length = %d, want %d (the enrichment bound)",
				len([]rune(got[0].Text)), len([]rune(bounded[0].Text)))
		}
	})
}

func equal(a, b []enrich.Requirement) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
