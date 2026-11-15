package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// trinethireProber probes a TriNet Hire company (board = the "<id>-<slug>" path segment). The
// listing is a server-rendered table with no JSON, so liveness is judged like the adapter: a live
// board links ≥1 posting. The <title> "Careers at <employer>" names the employer for the gate.
type trinethireProber struct{}

// dedupKey folds the name slug off, so two spellings of one company id are probed once (the same
// fold as sources.BoardDedupeKey on insert).
func (trinethireProber) dedupKey(board string) string { return foldedBoard("trinethire", board) }

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

// gohireProber probes a GoHire career site (board = the "<slug>-<code>" path segment) by its first
// listing page: a live site links ≥1 posting card; the <title> is "Jobs at <employer>". GoHire meters
// a request quota (~400, then 429s that outlast the run): harvest with -pace 0.1 and no more
// candidates than the quota holds (see sources/gohire.go).
type gohireProber struct{}

// dedupKey folds onto the site code, so two slugs in front of one site are probed once and the
// quota is spent on distinct sites only.
func (gohireProber) dedupKey(board string) string { return foldedBoard("gohire", board) }

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

// foldedBoard is the catalogue's own identity for a board (sources.BoardDedupeKey), so a prober
// dedups candidates exactly the way the insert will.
func foldedBoard(provider, board string) string {
	key, _ := sources.BoardDedupeKey(sources.CompanyEntry{Provider: provider, Board: board})
	return key
}
