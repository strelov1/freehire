package main

import (
	"context"
	"fmt"
	"regexp"
)

// hiringthingProber probes a HiringThing career site, whose board is the full careers host: the
// platform is white-labelled and one tenant is served under the vendor's domain and each
// reseller's, so a bare slug names no board. The site root renders the whole board as HTML (no
// JSON feed, no pagination), so liveness is judged as the ingest adapter reads the page: a live
// board links at least one posting permalink.
// The listing's <title> names an employer, but a board is as often a SITE as a company (one
// community, one dealership rooftop) while the seed names the operator, and they often
// disagree. The prober therefore reports no name and the seed's company labels the board.
type hiringthingProber struct{}

// hiringthingJobPattern is the posting permalink on a HiringThing listing: /job/<numeric id>,
// relative or absolute. It must not match the site's id-less /job-prefixed navigation, and it
// anchors at the start of the PATH so an id in another link's query string is not counted. Same
// shape as the ingest adapter, so an accepted board is one that adapter can crawl.
var hiringthingJobPattern = regexp.MustCompile(`^(?:https?://[^/]+)?/job/\d+`)

func (hiringthingProber) probe(ctx context.Context, c httpClient, host string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s/", host))
	if err != nil {
		return "", 0, nil
	}
	n := countMatchingLinks(root, hiringthingJobPattern)
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}
