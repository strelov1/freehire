package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// metacareers adapts Meta's career site (metacareers.com). It is a boardless single-company
// source: Fetch ignores e.Board and uses e.Company ("Meta"), like google/amazon. Meta's edge
// rejects Go's default TLS+HTTP/2 fingerprint, so in production the adapter is wired with the
// shared Chrome-fingerprint transport (fingerprintHTTP, see fingerprinthttp.go) rather than the
// shared client. The jobsearch sitemap enumerates job_details URLs; each job page server-renders
// an application/ld+json JobPosting, so title/description/date/location come from a per-job detail
// fetch under the shared bounded-concurrency pool, like the other detail-fetching adapters.

// metacareersHTTP is the transport metacareers needs: an XML sitemap plus HTML detail pages.
type metacareersHTTP interface {
	XMLGetter
	HTMLGetter
}

type metacareers struct {
	http metacareersHTTP
}

// NewMetaCareers builds the Meta adapter over the given HTTP client (the shared Chrome-fingerprint
// fingerprintHTTP in production).
func NewMetaCareers(c metacareersHTTP) Source { return metacareers{http: c} }

func (metacareers) Provider() string { return "meta" }

// boardless marks meta as a single-company source: its config entry omits board, and it is
// excluded from the source facet (redundant with the company filter), like google/amazon.
func (metacareers) boardless() {}

// metaSitemapURL is Meta's flat jobsearch sitemap: a <urlset> of job_details page URLs.
const metaSitemapURL = "https://www.metacareers.com/jobsearch/sitemap.xml"

// metaSitemapEntry is one <url> of the jobsearch sitemap: the job page URL and its
// last-modified date (used as a posted_at fallback).
type metaSitemapEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

func (m metacareers) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []metaSitemapEntry `xml:"url"`
	}
	if err := m.http.GetXML(ctx, metaSitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("meta: sitemap: %w", err)
	}

	return fetchDetails(sitemap.URLs, defaultDetailWorkers, func(entry metaSitemapEntry) (Job, bool) {
		return m.detail(ctx, e, entry)
	}), nil
}

// metaLDPosting is the slice of the page's schema.org JobPosting the adapter reads. The
// jobLocation address sub-object is deliberately omitted: Meta renders its
// addressLocality/Region/Country incorrectly (a repeated bogus value), while jobLocation[].name
// is reliable.
type metaLDPosting struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	DatePosted  string      `json:"datePosted"`
	JobLocation []metaPlace `json:"jobLocation"`
}

// metaPlace is one entry of the JobPosting jobLocation array. Only name is read; the address
// sub-object is omitted because Meta renders it incorrectly (see the type doc above).
type metaPlace struct {
	Name string `json:"name"`
}

// metaJobIDPattern captures the numeric id of a /job_details/<id> URL. Anchored to the end of
// the path (like successfactors' sfJobID) so it never matches a mid-path digit run.
var metaJobIDPattern = regexp.MustCompile(`/job_details/(\d+)/?$`)

// metaJobID extracts the native numeric posting id from a job page URL, "" when absent.
func metaJobID(loc string) string {
	return firstSubmatch(metaJobIDPattern, loc)
}

// detail fetches one job page and maps its ld+json JobPosting to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, or has no parseable id (which would collide on the
// dedup key) — so the caller skips just that posting.
func (m metacareers) detail(ctx context.Context, e CompanyEntry, entry metaSitemapEntry) (Job, bool) {
	id := metaJobID(entry.Loc)
	if id == "" {
		return Job{}, false
	}

	root, err := m.http.GetHTML(ctx, entry.Loc)
	if err != nil {
		return Job{}, false
	}
	var p metaLDPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := metaPrimaryLocation(p.JobLocation)
	posted := parseRFC3339(p.DatePosted)
	if posted == nil {
		posted = parseRFC3339(entry.LastMod)
	}

	return Job{
		ExternalID:  id,
		URL:         entry.Loc,
		Title:       strings.TrimSpace(p.Title),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      isRemote(location),
		PostedAt:    posted,
	}, true
}

// metaPrimaryLocation collapses a JobPosting's jobLocation array into the single Location
// string the normalized Job carries (and that the geography dictionary later parses into
// country/region facets). A Meta posting can list many cities; this takes the first named
// location. Returns "" when there is none, so enrichment derives location from the description.
func metaPrimaryLocation(locations []metaPlace) string {
	for _, l := range locations {
		if name := strings.TrimSpace(l.Name); name != "" {
			return name
		}
	}
	return ""
}
