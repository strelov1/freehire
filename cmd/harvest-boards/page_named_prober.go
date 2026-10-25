package main

import (
	"context"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

// pageNamedProber validates a board by running its source adapter, as adapterProber does, and
// then reads the employer's name from one page of the board — for a platform whose adapter takes
// the employer from the catalogue row rather than the platform, so the plain fallback would store
// the board id as the company (a "walnutfolks.zohorecruit.in" employer; see cmd/
// backfill-company-names for what such rows cost to repair afterwards). The name is also what
// lets a seed's company claim reject a guessed board that belongs to someone else.
type pageNamedProber struct {
	adapter adapterProber
	// nameURL is the page that names the employer; name reads it from the parsed document.
	nameURL func(board string) string
	name    func(root *html.Node) string
}

func newPageNamedProber(provider string, nameURL func(string) string, name func(*html.Node) string) pageNamedProber {
	return pageNamedProber{
		adapter: adapterProber{provider: provider, newSource: func() sources.Source { return adapterRegistry()[provider] }},
		nameURL: nameURL,
		name:    name,
	}
}

func (p pageNamedProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	_, n, err := p.adapter.probeSeeded(ctx, c, board, "")
	if err != nil || n == 0 {
		return "", 0, err
	}
	root, err := c.GetHTML(ctx, p.nameURL(board))
	if err != nil {
		return "", n, nil // the name is best effort; the board is live either way
	}
	return p.name(root), n, nil
}

// Darwinbox's listing API sits behind a Cloudflare edge only the adapter's Chrome-fingerprint
// transport passes, which is why it is probed through the adapter at all. The portal page, served
// to a non-browser client as a shell, names the tenant's legal entity in its <title> ("Ather
// Energy Limited"); the API names nobody.
var darwinboxProber = newPageNamedProber("darwinbox",
	func(board string) string { return "https://" + board + "/ms/candidatev2/main/careers/allJobs" },
	pageTitle)

// Zoho Recruit's careers page names the employer in og:site_name ("Walnut Folks"); its <title>
// is a sentence ("Careers at Walnut Folks Group - Confluencr & Intent Farm").
var zohoRecruitProber = newPageNamedProber("zohorecruit",
	func(board string) string { return "https://" + board + "/jobs/Careers" },
	func(root *html.Node) string { return metaProperty(root, "og:site_name") })

// PyjamaHR's API names no employer; the board's career page carries it as its <title>.
var pyjamahrProber = newPageNamedProber("pyjamahr",
	func(board string) string { return "https://jobs.pyjamahr.com/" + board },
	pageTitle)
