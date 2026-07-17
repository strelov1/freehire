package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// wpyoast adapts WordPress career sites built on the WP Job Manager plugin with a Yoast
// SEO sitemap (e.g. "careers.theplanetgroup.com"). The board is the career-site host. The
// Yoast sitemap index points to a "job_listing" sub-sitemap that enumerates the postings;
// each job page is server-rendered HTML carrying a schema.org JobPosting ld+json block, so
// the description comes from a per-job detail fetch (bounded-concurrency), like the other
// HTML detail adapters (successfactors, radancy).

// wpyoastHTTP is the transport wpyoast needs: an XML sitemap plus HTML detail pages.
type wpyoastHTTP interface {
	XMLGetter
	HTMLGetter
}

type wpyoast struct {
	http wpyoastHTTP
}

// NewWPYoast builds the WP/Yoast adapter over the given HTTP client.
func NewWPYoast(c wpyoastHTTP) Source { return wpyoast{http: c} }

func (wpyoast) Provider() string { return "wpyoast" }

func (s wpyoast) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// The Yoast sitemap index lists per-post-type sub-sitemaps; the postings live in the
	// "job_listing" one. Resolving it from the index (rather than guessing the filename)
	// keeps the adapter working across Yoast versions that name the file differently.
	jobSitemap, err := resolveSubSitemap(ctx, s.http, fmt.Sprintf("https://%s/sitemap.xml", e.Board), "job_listing")
	if err != nil {
		return nil, fmt.Errorf("wpyoast: %s: %w", e.Board, err)
	}
	if jobSitemap == "" {
		return nil, nil // no job_listing sub-sitemap → no postings, not an error
	}

	locs, err := sitemapJobLocs(ctx, s.http, jobSitemap, wpyoastJobID)
	if err != nil {
		return nil, fmt.Errorf("wpyoast: job sitemap %s: %w", e.Board, err)
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails or carries no JobPosting, so the caller skips just that posting.
func (s wpyoast) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p wpyoastPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()

	return Job{
		ExternalID:  wpyoastJobID(loc),
		URL:         loc,
		Title:       html.UnescapeString(p.Title),
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// wpyoastPosting is the schema.org JobPosting decoded from a WP/Yoast job page's
// application/ld+json block. jobLocation is a single Place (not an array).
type wpyoastPosting struct {
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	DatePosted         string      `json:"datePosted"`
	JobLocation        schemaPlace `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// wpyoastJobIDPattern captures the numeric posting id from a /job/<id>-<slug>/ URL.
var wpyoastJobIDPattern = regexp.MustCompile(`/job/(\d+)-`)

// wpyoastJobID extracts the native numeric posting id from a job page URL, or "" when the
// URL is not a job posting (so non-job sitemap entries are dropped).
func wpyoastJobID(loc string) string {
	return firstSubmatch(wpyoastJobIDPattern, loc)
}
