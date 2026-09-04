package sources

import (
	"context"
	"testing"
)

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
