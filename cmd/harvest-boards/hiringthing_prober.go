package main

import (
	"context"
	"fmt"
	"regexp"
)

// hiringthingProber probes a HiringThing career site, whose board is the full careers host —
// the platform is white-labelled and the same tenant is served under the vendor's domain and
// under each reseller's, so a bare slug names no board. The site root renders the whole board
// as HTML (there is no JSON feed and no pagination), so liveness is judged the way the ingest
// adapter reads the same page: a live board links at least one posting permalink.
//
// The listing's <title> does name an employer ("<name> Career Opportunities"), but not one the
// corroboration gate can stand on: on this platform a board is as often a SITE as a company —
// one senior-living community, one dealership rooftop, one hotel — so the title names the site
// while the seed names the operator that runs it. Measured over 120 live boards the two
// disagreed on 48% of them, almost all of that shape. The prober therefore reports no name and
// the seed's company labels the board.
type hiringthingProber struct{}

// hiringthingJobPattern is the posting permalink on a HiringThing listing: /job/<numeric id>,
// relative or absolute. It must not match the site's other /job-prefixed navigation, which
// carries no id, and it anchors at the start of the PATH so an id sitting in another link's
// query string is not counted — the same shape the ingest adapter enumerates by, so a board
// this accepts as live is one that adapter can crawl.
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
