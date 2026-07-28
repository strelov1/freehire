package sources

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"golang.org/x/net/html"
)

// instaffo adapts jobs.instaffo.com, a German hiring marketplace. Boardless (one site, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's employer from the page. The sitemap index points to a "jobs" sub-sitemap; each
// job page server-renders a schema.org JobPosting ld+json block, so the description comes
// from a per-job detail fetch (bounded-concurrency), like the other HTML detail adapters.
//
// Every posting is listed twice, once per locale, under one native id and with identical
// ld+json — the locale only swaps the surrounding chrome. instaffoJobID answers for the
// canonical /de/ URL alone, which drops each /en/ twin before it costs a fetch.

// instaffoHTTP is the transport instaffo needs: an XML sitemap plus HTML detail pages.
type instaffoHTTP interface {
	XMLGetter
	HTMLGetter
}

type instaffo struct {
	http instaffoHTTP
}

// NewInstaffo builds the Instaffo adapter over the given HTTP client.
func NewInstaffo(c instaffoHTTP) Source { return instaffo{http: c} }

func (instaffo) Provider() string { return "instaffo" }

func (instaffo) boardless() {}

func (instaffo) aggregator() {}

func (s instaffo) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	// The needle is the full "sitemap-jobs" stem, not "jobs": the site's own host carries
	// "jobs", so a bare needle matches every sub-sitemap and picks whichever comes first.
	jobSitemap, err := resolveSubSitemap(ctx, s.http, "https://jobs.instaffo.com/sitemap.xml", "sitemap-jobs")
	if err != nil {
		return nil, fmt.Errorf("instaffo: %w", err)
	}
	if jobSitemap == "" {
		return nil, nil
	}

	locs, err := sitemapJobLocs(ctx, s.http, jobSitemap, instaffoJobID)
	if err != nil {
		return nil, fmt.Errorf("instaffo: job sitemap: %w", err)
	}

	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, loc)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or lacks an employer (which would break
// the company slug). The ld+json carries no url, so the page loc is the canonical link.
func (s instaffo) detail(ctx context.Context, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p instaffoPosting
	if !ldJobPosting(root, &p) || p.HiringOrganization.Name == "" {
		return Job{}, false
	}

	// Instaffo never retires a posting from its sitemap — validThrough is stamped a year out
	// on every fetch, so it carries no signal, and the 48h unseen sweep never fires because
	// the posting never goes unseen. Age is the only staleness signal, so it gates on the way
	// in: a posting older than the window would otherwise sit open in the catalogue forever.
	posted := parseDate(p.DatePosted)
	if posted == nil || time.Since(*posted) > instaffoMaxAge {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()

	return Job{
		ExternalID:  instaffoJobID(loc),
		URL:         loc,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location) || isRemote(p.Title),
		PostedAt:    posted,
	}, true
}

// instaffoMaxAge is how old a posting may be and still be ingested. Instaffo keeps postings
// in its sitemap indefinitely (the catalogue holds ads from two years back), and nothing
// downstream can close one, so this is the only thing standing between the catalogue and a
// permanent tail of dead ads. A year is deliberately generous: German mid-market vacancies
// do legitimately stay open for months.
const instaffoMaxAge = 365 * 24 * time.Hour

// instaffoPosting is the schema.org JobPosting decoded from an Instaffo job page. jobLocation
// is a single Place (not an array), and datePosted is date-only.
type instaffoPosting struct {
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	DatePosted         string      `json:"datePosted"`
	JobLocation        schemaPlace `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// instaffoJobIDPattern captures the posting slug from a canonical /de/job/<slug> URL, where
// the slug ends in the native hex id. The locale is part of the pattern on purpose: it is
// what makes the /en/ twin of every posting answer "" and drop out.
var instaffoJobIDPattern = regexp.MustCompile(`/de/job/([^/?#]+-[0-9a-f]{12})$`)

// instaffoJobID extracts the native posting id from a job page URL, or "" when the URL is not
// a canonical job posting (so locale twins and non-job sitemap entries are dropped).
func instaffoJobID(loc string) string {
	return firstSubmatch(instaffoJobIDPattern, loc)
}
