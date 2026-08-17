package sources

import (
	"context"
	"fmt"
)

// uber adapts Uber's public careers site (jobs.uber.com), a Next.js/RSC app whose HTML routes
// sit behind a Cloudflare bot-challenge a plain HTTP client cannot pass — so it is wired with
// the shared Chrome-fingerprint transport (fingerprintHTTP, see fingerprinthttp.go) rather than
// the shared client, like meta/bayt/gulftalent. The site's own sitemap enumerates every live
// posting (the XML endpoint is not behind the challenge); each job page server-renders a full
// schema.org JobPosting ld+json block, including the HTML description, so one sitemap fetch plus
// a per-job detail fetch assembles every Job — no search API needed. Single-company, boardless
// (no per-tenant board id).
type uber struct {
	http uberHTTP
}

// uberHTTP is the transport uber needs: an XML sitemap plus HTML detail pages.
type uberHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewUber builds the Uber adapter over the given HTTP client (the shared Chrome-fingerprint
// fingerprintHTTP in production — see registry.go).
func NewUber(c uberHTTP) Source { return uber{http: c} }

func (uber) Provider() string { return "uber" }

// uber is single-company, so its config entries carry no board.
func (uber) boardless() {}

// uberSitemapURL is jobs.uber.com's flat job sitemap: a <urlset> of /en/jobs/<id>/ detail URLs.
const uberSitemapURL = "https://jobs.uber.com/en/jobs/sitemap.xml"

func (u uber) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	sitemap, err := getSitemap(ctx, u.http, uberSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("uber: sitemap: %w", err)
	}

	// The sitemap's lastmod is carried into detail as a posted_at fallback.
	return fetchDetails(sitemap.URLs, defaultDetailWorkers, func(entry sitemapLoc) (Job, bool) {
		return u.detail(ctx, e, entry)
	}), nil
}

// uberPosting is the schema.org JobPosting slice this adapter reads off a job detail page.
type uberPosting struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	DatePosted     string `json:"datePosted"`
	EmploymentType string `json:"employmentType"`
	Identifier     struct {
		Value string `json:"value"`
	} `json:"identifier"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// detail fetches one job page and maps its ld+json JobPosting to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, or has no identifier (which would collide on the
// dedup key) — so the caller skips just that posting.
func (u uber) detail(ctx context.Context, e CompanyEntry, entry sitemapLoc) (Job, bool) {
	root, err := u.http.GetHTML(ctx, entry.Loc)
	if err != nil {
		return Job{}, false
	}
	var p uberPosting
	if !ldJobPosting(root, &p) || p.Identifier.Value == "" {
		return Job{}, false
	}

	var location string
	if len(p.JobLocation) > 0 {
		location = p.JobLocation[0].Address.Location()
	}
	posted := parseRFC3339OrDate(p.DatePosted)
	if posted == nil {
		posted = parseRFC3339(entry.LastMod)
	}

	return Job{
		ExternalID:  p.Identifier.Value,
		URL:         entry.Loc,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		// No structured jobLocationType is present in Uber's ld+json, so remote is the same
		// location-text heuristic meta's adapter falls back to — never WorkMode, which is
		// reserved for a platform's own structured signal.
		Remote:         isRemote(location),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       posted,
	}, true
}
