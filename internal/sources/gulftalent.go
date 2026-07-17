package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// gulftalent adapts GulfTalent.com, a major Gulf job board. It is a boardless multi-company
// aggregator: one crawl enumerates the site's sitemap index, follows the job-posting shards
// (sitemap_jx0NN.xml — the jl/jc/co shards are category/company/course pages, not postings),
// fetches each job-detail page, and reads its self-contained schema.org JobPosting — so the
// employer comes from the posting (hiringOrganization), not a configured entry. GulfTalent's
// Akamai edge 403s Go's default TLS+HTTP/2 fingerprint, so in production the adapter is wired with
// the shared Chrome-fingerprint transport (fingerprintHTTP) rather than the shared client. Keyless.

// gulftalentHTTP is the transport gulftalent needs: XML sitemaps plus HTML detail pages.
type gulftalentHTTP interface {
	XMLGetter
	HTMLGetter
}

type gulftalent struct {
	http gulftalentHTTP
}

// NewGulfTalent builds the GulfTalent adapter over the given HTTP client (the shared
// Chrome-fingerprint fingerprintHTTP in production).
func NewGulfTalent(c gulftalentHTTP) Source { return gulftalent{http: c} }

func (gulftalent) Provider() string { return "gulftalent" }

// boardless marks gulftalent as having no per-tenant board id: one crawl covers the whole site.
func (gulftalent) boardless() {}

// aggregator keeps gulftalent in the source facet: one crawl aggregates postings from many
// companies, so filtering by source=gulftalent is meaningful (not redundant with the company
// filter).
func (gulftalent) aggregator() {}

const (
	gulftalentSitemapURL = "https://www.gulftalent.com/sitemap.xml"
	// gulftalentJobShardMarker selects the job-posting sitemap shards (sitemap_jx0NN.xml) from the
	// index; the jl/jc/co shards are category, company, and course pages that carry no JobPosting.
	gulftalentJobShardMarker = "_jx"
	// gulftalentDetailWorkers bounds the detail fan-out below the shared defaultDetailWorkers (8):
	// GulfTalent's Akamai edge throttles fast bursts and the fingerprint transport does not retry,
	// so a wide fan-out across a large catalogue would silently drop postings.
	gulftalentDetailWorkers = 4
)

// gulftalentJobIDPattern captures the numeric id at the end of a GulfTalent job-detail path
// (/<country>/jobs/<slug>-<id>). It requires the /jobs/ segment so a /companies/ or
// /jobs/category/ URL is not mistaken for a posting, and anchors the id to the end.
var gulftalentJobIDPattern = regexp.MustCompile(`/jobs/[^/]+-(\d+)/?$`)

// gulftalentJobID extracts the native GulfTalent posting id from a job-detail URL, "" when the URL
// is not a job-detail page or carries no trailing id. Any query string or fragment is stripped
// first so a URL with a suffix still matches.
func gulftalentJobID(loc string) string {
	return firstSubmatch(gulftalentJobIDPattern, trimURLSuffix(loc))
}

// gulftalentLDPosting is the slice of the page's schema.org JobPosting the adapter reads.
// jobLocation is a single Place whose address carries the country the geography dictionary reads.
type gulftalentLDPosting struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	DatePosted  string          `json:"datePosted"`
	HiringOrg   gulftalentOrg   `json:"hiringOrganization"`
	JobLocation gulftalentPlace `json:"jobLocation"`
}

type gulftalentOrg struct {
	Name string `json:"name"`
}

type gulftalentPlace struct {
	Address gulftalentAddress `json:"address"`
}

type gulftalentAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressCountry  string `json:"addressCountry"`
}

func (g gulftalent) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	index, err := getSitemap(ctx, g.http, gulftalentSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("gulftalent: sitemap index: %w", err)
	}

	var links []string
	for _, s := range index.Sitemaps {
		if !strings.Contains(s.Loc, gulftalentJobShardMarker) {
			continue // only the job-posting shards carry JobPostings
		}
		shard, err := getSitemap(ctx, g.http, s.Loc)
		if err != nil {
			continue // a single unreadable shard just drops its slice of the catalogue this run
		}
		for _, u := range shard.URLs {
			if gulftalentJobID(u.Loc) != "" {
				links = append(links, u.Loc)
			}
		}
	}

	return fetchDetails(links, gulftalentDetailWorkers, func(link string) (Job, bool) {
		return g.detail(ctx, link)
	}), nil
}

// detail fetches one job page and maps its ld+json JobPosting to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, has no resolvable employer (company-less), or has
// no parseable id — so the caller skips just that posting.
func (g gulftalent) detail(ctx context.Context, link string) (Job, bool) {
	id := gulftalentJobID(link)
	if id == "" {
		return Job{}, false
	}
	root, err := g.http.GetHTML(ctx, link)
	if err != nil {
		return Job{}, false
	}
	var p gulftalentLDPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	company := strings.TrimSpace(p.HiringOrg.Name)
	if company == "" {
		return Job{}, false
	}

	location := joinNonEmpty(
		strings.TrimSpace(p.JobLocation.Address.AddressLocality),
		strings.TrimSpace(p.JobLocation.Address.AddressCountry),
	)
	return Job{
		ExternalID:  id,
		URL:         link,
		Title:       strings.TrimSpace(p.Title),
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}
