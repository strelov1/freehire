package sources

import (
	"context"
	"strings"
	"testing"
)

func TestSanitizeJSONControlCharsEscapesInsideStringOnly(t *testing.T) {
	raw := []byte("{\"a\":\"line one\nline two\",\n \"b\":1}")
	got := sanitizeJSONControlChars(raw)
	want := "{\"a\":\"line one\\nline two\",\n \"b\":1}"
	if string(got) != want {
		t.Errorf("sanitizeJSONControlChars = %q, want %q", got, want)
	}
}

func TestSanitizeJSONControlCharsSkipsEscapedBackslash(t *testing.T) {
	// A literal backslash-quote inside the string must not flip inString off early.
	raw := []byte(`{"a":"quote: \" then a real newline` + "\n" + `end"}`)
	got := sanitizeJSONControlChars(raw)
	if strings.Contains(string(got), "\n") {
		t.Errorf("sanitizeJSONControlChars left a raw newline unescaped: %q", got)
	}
}

// Live-verified on cryptocurrencyjobs.co: its JobPosting ld+json embeds the description with
// raw, unescaped newlines, which Go's strict json.Unmarshal rejects ("invalid character '\n'
// in string literal") — a lenient decoder (e.g. Python's json.loads(strict=False)) accepts it.
func TestLdJobPostingToleratesRawNewlineInDescription(t *testing.T) {
	page := `<html><head><script type="application/ld+json">` +
		"{\"@context\":\"https://schema.org/\",\"@type\":\"JobPosting\",\"title\":\"Engineer\"," +
		"\"description\":\"Line one.\nLine two.\"}" +
		`</script></head><body>page</body></html>`
	fake := (&routedHTTP{}).route("/job", page)
	node, err := fake.GetHTML(context.Background(), "https://example.com/job")
	if err != nil {
		t.Fatalf("GetHTML: %v", err)
	}
	var p struct {
		Description string `json:"description"`
	}
	if !ldJobPosting(node, &p) {
		t.Fatal("ldJobPosting returned false, want it to tolerate the raw newline")
	}
	if p.Description != "Line one.\nLine two." {
		t.Errorf("Description = %q", p.Description)
	}
}
