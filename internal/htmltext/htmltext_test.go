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

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"under limit", "hello", 10, "hello"},
		{"exact limit", "hello", 5, "hello"},
		{"over limit", "hello world", 5, "hello"},
		{"does not split a multi-byte rune", "h→llo", 2, "h→"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateRunes(tc.in, tc.limit); got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
		})
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
