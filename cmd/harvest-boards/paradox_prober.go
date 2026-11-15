package main

import (
	"context"
	"net/url"
	"strings"
)

// paradoxProber validates a Paradox board ("<host>/<site>") by asking the site's public job
// list how many postings it has, one request per candidate. It mirrors the listing request of
// internal/ingest/sources/paradox.go, which is unexported. total_jobs counts the whole site
// whatever the page size, so a one-row page settles liveness, where the adapter fallback would
// hydrate every posting. It reports no employer name: the platform's is the BRAND ("Wingstop")
// while a site usually belongs to one franchisee, so using it would reject live boards on the
// name gate; the seed's supplies it.
type paradoxProber struct{}

func (paradoxProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	host, site, ok := strings.Cut(strings.ToLower(board), "/")
	if !ok || !strings.HasSuffix(host, ".paradox.ai") || site == "" || strings.Contains(site, "/") {
		return "", 0, nil
	}
	q := url.Values{"compsite_slug": {site}, "limit": {"1"}, "page": {"1"}}
	var page struct {
		Success   bool `json:"success"`
		TotalJobs int  `json:"total_jobs"`
	}
	// A site with no public job list (or no such company) answers 404, which the client surfaces
	// as an error; for harvest that is simply "not a live board". The API answers 404 to a
	// request without X-Requested-With too, so the header is not optional.
	headers := map[string]string{"X-Requested-With": "XMLHttpRequest"}
	if err := c.GetJSONWithHeaders(ctx, "https://"+host+"/api/job-list-site-ui/search-job?"+q.Encode(), headers, &page); err != nil || !page.Success {
		return "", 0, nil
	}
	return "", page.TotalJobs, nil
}
