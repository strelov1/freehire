package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// thehub adapts thehub.io, a Nordic startup job board. Boardless (one site, no per-tenant
// board) and multi-company, so it stays in the source facet and takes each posting's
// company from the page. The Yoast-style sitemap index points to a "jobs" sub-sitemap that
// enumerates the postings; each job page server-renders a schema.org JobPosting ld+json
// block, so the description comes from a per-job detail fetch (bounded-concurrency), like
// the other HTML detail adapters.

// thehubHTTP is the transport thehub needs: an XML sitemap plus HTML detail pages.
type thehubHTTP interface {
	XMLGetter
	HTMLGetter
}

type thehub struct {
	http thehubHTTP
}

// NewTheHub builds the TheHub adapter over the given HTTP client.
func NewTheHub(c thehubHTTP) Source { return thehub{http: c} }

func (thehub) Provider() string { return "thehub" }

func (thehub) boardless() {}

func (thehub) aggregator() {}

func (s thehub) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	jobSitemap, err := resolveSubSitemap(ctx, s.http, "https://thehub.io/sitemap.xml", "jobs")
	if err != nil {
		return nil, fmt.Errorf("thehub: %w", err)
	}
	if jobSitemap == "" {
		return nil, nil
	}

	locs, err := sitemapJobLocs(ctx, s.http, jobSitemap, thehubJobID)
	if err != nil {
		return nil, fmt.Errorf("thehub: job sitemap: %w", err)
	}

	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, loc)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or lacks a company (which would break
// the slug). The JSON-LD url is null, so the page loc is the canonical link.
func (s thehub) detail(ctx context.Context, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p thehubPosting
	if !ldJobPosting(root, &p) || p.HiringOrganization.Name == "" {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()

	return Job{
		ExternalID:  thehubJobID(loc),
		URL:         loc,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// thehubPosting is the schema.org JobPosting decoded from a TheHub job page. jobLocation is
// a single Place (not an array).
type thehubPosting struct {
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	DatePosted         string      `json:"datePosted"`
	JobLocation        schemaPlace `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// thehubJobIDPattern captures the posting id from a /jobs/<id> URL (a 24-hex object id).
var thehubJobIDPattern = regexp.MustCompile(`/jobs/([0-9a-fA-F]{12,})`)

// thehubJobID extracts the native posting id from a job page URL, or "" when the URL is not
// a job posting (so non-job sitemap entries are dropped).
func thehubJobID(loc string) string {
	return firstSubmatch(thehubJobIDPattern, loc)
}
