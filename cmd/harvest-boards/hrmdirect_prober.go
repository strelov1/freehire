package main

import (
	"context"
	"fmt"
	"regexp"
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

// hrmdirectJobLink matches a posting link on the listing. Both ids are required: a link
// carrying only req is the filter form's, not a posting's.
var hrmdirectJobLink = regexp.MustCompile(`job-opening\.php\?req=\d+&req_loc=\d+`)

func (hrmdirectProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx,
		fmt.Sprintf("https://%s.hrmdirect.com/employment/job-openings.php?search=true", slug))
	if err != nil {
		return "", 0, nil
	}
	return "", countMatchingLinks(root, hrmdirectJobLink), nil
}
