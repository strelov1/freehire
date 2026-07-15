package sources

import (
	"golang.org/x/net/html"
	"strings"
	"testing"
)

// ldPage wraps a raw ld+json body in a minimal HTML page.
func ldPage(body string) *html.Node {
	n, _ := html.Parse(strings.NewReader(
		`<html><body><script type="application/ld+json">` + body + `</script></body></html>`))
	return n
}

func TestLDJobPostingShapes(t *testing.T) {
	posting := `{"@type":"JobPosting","title":"Role","identifier":{"value":"x1"}}`
	cases := map[string]string{
		"bare object":    posting,
		"array of nodes": `[{"@type":"Organization","name":"Co"},` + posting + `]`,
		"graph wrapper":  `{"@context":"https://schema.org","@graph":[{"@type":"WebSite"},` + posting + `]}`,
	}
	for name, body := range cases {
		var got struct {
			Title      string `json:"title"`
			Identifier struct {
				Value string `json:"value"`
			} `json:"identifier"`
		}
		if !ldJobPosting(ldPage(body), &got) {
			t.Errorf("%s: ldJobPosting = false, want true", name)
			continue
		}
		if got.Title != "Role" || got.Identifier.Value != "x1" {
			t.Errorf("%s: decoded %+v, want title=Role value=x1", name, got)
		}
	}
	// A page with no JobPosting node anywhere yields false.
	var sink struct{}
	if ldJobPosting(ldPage(`[{"@type":"Organization"}]`), &sink) {
		t.Error("ldJobPosting on non-JobPosting array = true, want false")
	}
}
