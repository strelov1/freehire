package reqextract

import (
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

// MaskPreferred blanks the WORDS of a preferred-only section and nothing else, so a
// test asserts on what a downstream matcher can still SEE rather than on an exact
// string: the description's surviving text, transliterated and reduced to words.
func words(descriptionHTML string) string {
	doc, err := xhtml.Parse(strings.NewReader(descriptionHTML))
	if err != nil {
		return "<unparseable>"
	}
	return normalizeHeading(textOf(doc))
}

func TestMaskPreferred(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string // the words that survive, lowercased, in order
	}{
		{
			name: "a preferred section, heading included, is blanked; the required one survives",
			html: `<h3>Requirements</h3><ul><li>CISSP</li></ul>` +
				`<h3>Nice to have</h3><ul><li>PMP</li></ul>`,
			want: "requirements cissp",
		},
		{
			name: "a hungarian preferred section is recognized",
			html: `<h3>Elvárások</h3><p>BSc diploma.</p><h3>Előnyt jelent</h3><p>PhD.</p>`,
			want: "elvarasok bsc diploma",
		},
		{
			name: "a preferred section made of paragraphs, not a list, is still masked",
			html: `<h3>Preferred qualifications</h3><p>C1 English.</p>`,
			want: "",
		},
		{
			name: "a required heading after a preferred one reopens the text",
			html: `<h3>Nice to have</h3><p>PMP.</p><h3>Requirements</h3><p>CISSP.</p>`,
			want: "requirements cissp",
		},
		{
			name: "a closing heading after a preferred one reopens the text",
			html: `<h3>Bonus</h3><p>PMP.</p><h3>What we offer</h3><p>Stock options.</p>`,
			want: "what we offer stock options",
		},
		{
			name: "a description with no preferred heading is returned byte-identical",
			html: `<h3>Requirements</h3><ul><li>CISSP</li></ul>`,
			want: "requirements cissp",
		},
		{
			name: "plain text with no markup is left alone",
			html: "Requires an active CISSP.",
			want: "requires an active cissp",
		},
		{name: "empty", html: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := words(MaskPreferred(tc.html)); got != tc.want {
				t.Errorf("words(MaskPreferred) = %q, want %q", got, tc.want)
			}
		})
	}
}

// A description that names no preferred section must come back unchanged, not merely
// equivalent: the deterministic matchers downstream read punctuation as structure, and
// a re-rendered description is a second chance for that structure to shift.
func TestMaskPreferredLeavesAnUnaffectedDescriptionByteIdentical(t *testing.T) {
	for _, in := range []string{
		`<h3>Requirements</h3><ul><li>CISSP &amp; PMP</li></ul>`,
		"English, B2 level required.",
		"",
	} {
		if got := MaskPreferred(in); got != in {
			t.Errorf("MaskPreferred(%q) = %q, want it unchanged", in, got)
		}
	}
}

// Re-rendering a parsed document is not a no-op on its TEXT: x/net/html escapes an
// apostrophe, a quote and a bare ">" that the description carried literally. The
// matchers downstream read that text, and `bachelor's` spelled `bachelor&#39;s` matches
// nothing — which broke exactly the postings this masker exists for, since only a
// posting WITH a preferred section is ever re-rendered. Found against prod rows, not
// here: every unit test was green.
func TestMaskPreferredDoesNotReescapeText(t *testing.T) {
	const preferredSection = `<h3>Nice to have</h3><p>Kubernetes.</p>`
	cases := []struct{ name, html, want string }{
		{
			name: "apostrophe",
			html: `<h3>Requirements</h3><p>Bachelor's degree required.</p>` + preferredSection,
			want: `Bachelor's degree required.`,
		},
		{
			name: "quotes and a bare greater-than",
			html: `<h3>Requirements</h3><p>A "senior" engineer, 5 > 3 years.</p>` + preferredSection,
			want: `A "senior" engineer, 5 > 3 years.`,
		},
		{
			name: "an ampersand the source already escaped stays escaped",
			html: `<h3>Requirements</h3><p>R&amp;D experience.</p>` + preferredSection,
			want: `R&amp;D experience.`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaskPreferred(tc.html); !strings.Contains(got, tc.want) {
				t.Errorf("MaskPreferred did not preserve %q; got %q", tc.want, got)
			}
		})
	}
}
