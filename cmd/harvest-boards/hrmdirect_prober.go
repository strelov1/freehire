package main

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"

	"golang.org/x/net/html"
)

// hrmdirectProber validates an HRM Direct board "<slug>" by counting the postings its listing
// links ("<slug>.hrmdirect.com/employment/job-openings.php?search=true" — the bare path renders
// the filter form above an empty result, so the query is what makes the board answer).
//
// It reports no company name. The heading a tenant puts on its career site is free text, not an
// employer field: sampled live it reads "Current Openings" or "Careers and <name>" about as
// often as a name, and some tenants leave it out entirely. Reporting it would gate every board
// on a string the platform never promised was a company, so the seed's own name labels the
// board instead — the same reading careerplugProber takes.
type hrmdirectProber struct{}

// hrmdirectPostingID matches one of the two ids in a posting link, mirroring the adapter's own
// rule (internal/ingest/sources/hrmdirect.go): a posting is keyed by the PAIR (req, req_loc),
// and both must be numeric.
var hrmdirectPostingID = regexp.MustCompile(`^\d+$`)

func (hrmdirectProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx,
		fmt.Sprintf("https://%s.hrmdirect.com/employment/job-openings.php?search=true", slug))
	if err != nil {
		return "", 0, nil
	}
	// Counted by (req, req_loc) rather than by link, so the number reported is postings — the
	// same thing the adapter would ingest — and not however many times the page links them.
	ids := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					if id, ok := hrmdirectPostingRef(a.Val); ok {
						ids[id] = true
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return "", len(ids), nil
}

// hrmdirectPostingRef reads a listing link's "<req>-<req_loc>" posting id, reporting ok=false
// when the link is not a posting permalink. It decodes the query rather than pattern-matching
// it, so the filter form's own "?req=" link and any future parameter order both resolve right.
func hrmdirectPostingRef(href string) (string, bool) {
	u, err := url.Parse(href)
	if err != nil || path.Base(u.Path) != "job-opening.php" {
		return "", false
	}
	q := u.Query()
	req, loc := q.Get("req"), q.Get("req_loc")
	if !hrmdirectPostingID.MatchString(req) || !hrmdirectPostingID.MatchString(loc) {
		return "", false
	}
	return req + "-" + loc, true
}
