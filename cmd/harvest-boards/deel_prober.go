package main

import (
	"context"
	"fmt"
	"regexp"
)

// deelJobLocPattern matches a Deel sitemap entry that is a job-detail page (the same shape
// the adapter keys off), distinguishing a live board from the board's empty shell sitemap.
var deelJobLocPattern = regexp.MustCompile(`/job-details/`)

// deelProber probes a Deel ATS tenant by its per-board sitemap (the adapter's own tenant
// validation): a sitemap listing ≥1 /job-details/ URL is a live board. The sitemap carries
// no company name, so it falls back to the slug.
type deelProber struct{}

func (deelProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://jobs.deel.com/%s/sitemap.xml", slug), &sitemap); err != nil {
		return "", 0, nil
	}
	n := 0
	for _, u := range sitemap.URLs {
		if deelJobLocPattern.MatchString(u.Loc) {
			n++
		}
	}
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}
