package htmltext

import (
	"strings"
	"testing"
)

func TestToText_StripsTags(t *testing.T) {
	got := ToText("<p>Hello <strong>world</strong></p>")
	if got == "" {
		t.Fatal("ToText returned empty")
	}
	if containsAngle(got) {
		t.Errorf("ToText left HTML tags: %q", got)
	}
	if !contains(got, "Hello") || !contains(got, "world") {
		t.Errorf("ToText dropped content: %q", got)
	}
}

func TestToTextFull_PreservesPlainTextLines(t *testing.T) {
	for _, input := range []string{
		"We are hiring in Canada\nBenefits include paid leave.",
		"We are hiring in Canada\nBenefits include < 5 office visits a year.",
		"1 < 2 and 3 > 1\nNext line",
	} {
		if got := ToTextFull(input); got != input {
			t.Errorf("ToTextFull(%q) = %q, want unchanged plain text", input, got)
		}
	}
}

func TestToTextFull_ExtractsVisibleTextWithoutFormatting(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"inline emphasis", "<p>Candidates <strong>based in</strong> Canada.</p>", "Candidates based in Canada."},
		{"heading", "<h2>Candidates based in Canada.</h2>", "Candidates based in Canada."},
		{"entities", "<p>Canada &amp; the US &lt; 5 visits.</p>", "Canada & the US < 5 visits."},
		{"hidden content", "<head><title>Not visible</title></head><script>bad()</script><style>body{}</style><p>Canada</p>", "Canada"},
		{"text node newlines", "<p>Canada\nBenefits</p>", "Canada\nBenefits"},
		{"literal asterisks", "<p>Can*ada</p>", "Can*ada"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToTextFull(tt.input); got != tt.want {
				t.Errorf("ToTextFull(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToTextFull_PreservesContentBeyondRequestLimit(t *testing.T) {
	input := "<p>" + strings.Repeat("界", maxInputRunes) + "</p><p>Hello <strong>world</strong></p>"
	got := ToTextFull(input)
	if strings.Count(got, "界") != maxInputRunes || !strings.Contains(got, "Hello world") {
		t.Error("ToTextFull dropped content from the full description")
	}
	if containsAngle(got) {
		t.Error("ToTextFull left HTML tags")
	}
	if strings.Contains(ToText(input), "Hello") {
		t.Error("ToText no longer caps request-facing conversion")
	}
}

func TestToMarkdown_PreservesListStructure(t *testing.T) {
	got := ToMarkdown("<ul><li>first</li><li>second</li></ul>")
	if !contains(got, "first") || !contains(got, "second") {
		t.Errorf("ToMarkdown dropped content: %q", got)
	}
	// A Markdown list marker for at least one item.
	if !contains(got, "- first") && !contains(got, "* first") {
		t.Errorf("ToMarkdown did not render a list marker: %q", got)
	}
	if containsAngle(got) {
		t.Errorf("ToMarkdown left HTML tags: %q", got)
	}
}

func TestToText_BoundsVeryLargeInput(t *testing.T) {
	// Far larger (by rune count, ignoring markup) than maxInputRunes.
	huge := "<p>" + strings.Repeat("word ", maxInputRunes) + "</p>"
	got := ToText(huge)
	if n := len([]rune(got)); n > maxInputRunes {
		t.Errorf("ToText output has %d runes, want <= %d (maxInputRunes)", n, maxInputRunes)
	}
}

func TestResultCache_EvictsLeastRecentlyUsedOverCapacity(t *testing.T) {
	c := newCache(2)
	c.put("a", "1")
	c.put("b", "2")
	c.get("a")      // "a" is now more recently used than "b"
	c.put("c", "3") // over capacity: evicts "b", the least recently used

	if _, ok := c.get("b"); ok {
		t.Error("expected \"b\" to have been evicted")
	}
	if v, ok := c.get("a"); !ok || v != "1" {
		t.Errorf("get(a) = %q, %v; want 1, true", v, ok)
	}
	if v, ok := c.get("c"); !ok || v != "3" {
		t.Errorf("get(c) = %q, %v; want 3, true", v, ok)
	}
}

func containsAngle(s string) bool { return contains(s, "<") || contains(s, ">") }

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestToTextFullDecodesEntitiesWithoutMarkup(t *testing.T) {
	if got := ToTextFull("Canada &amp; US&#10;Benefits"); got != "Canada & US\nBenefits" {
		t.Errorf("visible text = %q", got)
	}
}
