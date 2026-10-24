package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// trinethireProber probes a TriNet Hire company (board = the "<id>-<slug>" path segment). The
// listing is one server-rendered table with no JSON behind it, so liveness is judged the way
// the ingest adapter reads the same page: a live board links at least one posting. The page's
// <title> is "Careers at <employer>", the platform's own rendering of the company name, so it
// names the employer for the corroboration gate.
type trinethireProber struct{}

// trinethireJobPattern is a posting link on the listing: /companies/<board>/jobs/<numeric id>-…
var trinethireJobPattern = regexp.MustCompile(`^/companies/[^/]+/jobs/\d+`)

func (trinethireProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://app.trinethire.com/companies/%s/jobs", board))
	if err != nil {
		return "", 0, nil
	}
	n := countMatchingLinks(root, trinethireJobPattern)
	if n == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(strings.TrimPrefix(pageTitle(root), "Careers at ")), n, nil
}

// gohireProber probes a GoHire career site (board = the "<slug>-<code>" path segment) by its
// first listing page: a live site links at least one posting card. The <title> is "Jobs at
// <employer>". GoHire meters a request quota (~400, then 429s that outlast the run), so
// harvest it with -pace 0.1 and no more candidates than the quota holds (see sources/gohire.go).
type gohireProber struct{}

// gohireJobPattern is a posting link on the listing: jobs.gohire.io/<board>/<slug>-<id>/.
var gohireJobPattern = regexp.MustCompile(`^https://jobs\.gohire\.io/[^/]+/[^/]*-\d+/?$`)

func (gohireProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://jobs.gohire.io/%s/?page=1", board))
	if err != nil {
		return "", 0, nil
	}
	n := countMatchingLinks(root, gohireJobPattern)
	if n == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(strings.TrimPrefix(pageTitle(root), "Jobs at ")), n, nil
}
