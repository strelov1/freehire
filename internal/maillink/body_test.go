package maillink

import (
	"strings"
	"testing"
)

func TestReadableBody_PrefersNonWhitespaceText(t *testing.T) {
	got := readableBody("Thanks for applying, we'll be in touch.", "<p>ignored html</p>")
	if got != "Thanks for applying, we'll be in touch." {
		t.Fatalf("plain-text part should be used verbatim, got %q", got)
	}
}

func TestReadableBody_HTMLOnlyStripsToReadableText(t *testing.T) {
	html := `<html><head><style>.x{color:red}</style></head><body>` +
		`<p>We regret to inform you that we have decided not to proceed.</p></body></html>`
	got := readableBody("", html)
	if got == "" {
		t.Fatal("HTML-only body must not classify to an empty string")
	}
	if strings.Contains(got, "<") || strings.Contains(got, "color:red") {
		t.Fatalf("readable body should be tag/style-free, got %q", got)
	}
	if !strings.Contains(got, "decided not to proceed") {
		t.Fatalf("readable body should carry the message text, got %q", got)
	}
}

func TestReadableBody_WhitespaceOnlyTextFallsBackToHTML(t *testing.T) {
	got := readableBody("   \n\t ", "<p>Interview invitation for next Tuesday.</p>")
	if !strings.Contains(got, "Interview invitation") {
		t.Fatalf("whitespace-only text should fall back to HTML, got %q", got)
	}
}

func TestReadableBody_BothEmptyYieldsEmpty(t *testing.T) {
	if got := readableBody("", ""); got != "" {
		t.Fatalf("both parts empty should yield empty body, got %q", got)
	}
}
