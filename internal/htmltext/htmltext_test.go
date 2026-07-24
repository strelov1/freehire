package htmltext

import "testing"

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

func containsAngle(s string) bool { return contains(s, "<") || contains(s, ">") }

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
