package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// compleo adapts jobs.compleo.app, a Brazilian ATS. The board is the tenant's path segment
// (e.g. "wafx"). Compleo publishes one sitemap for the whole platform rather than one per
// tenant, so a board is a slice of it: the postings under that tenant's segment. Each job page
// server-renders a schema.org JobPosting ld+json, so the description comes from a per-job
// detail fetch (bounded-concurrency), like the other HTML detail adapters.
//
// The platform is mostly non-technical hiring — a board is worth adding only when the tenant
// itself is (an IT consultancy, say), which is why boards are curated rather than harvested
// wholesale.

// compleoHTTP is the transport compleo needs: an XML sitemap plus HTML detail pages.
type compleoHTTP interface {
	XMLGetter
	HTMLGetter
}

type compleo struct {
	http compleoHTTP
}

// NewCompleo builds the Compleo adapter over the given HTTP client.
func NewCompleo(c compleoHTTP) Source { return compleo{http: c} }

func (compleo) Provider() string { return "compleo" }

func (s compleo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	locs, err := sitemapJobLocs(ctx, s.http, "https://jobs.compleo.app/sitemap.xml", compleoJobID)
	if err != nil {
		return nil, fmt.Errorf("compleo: sitemap: %w", err)
	}

	// The sitemap covers every tenant, so keep this board's postings. The prefix carries both
	// slashes: "wafx" must not swallow "wafxpro".
	prefix := "https://jobs.compleo.app/" + e.Board + "/jobdetail/"
	var mine []string
	for _, loc := range locs {
		if strings.HasPrefix(loc, prefix) {
			mine = append(mine, loc)
		}
	}

	return fetchDetails(mine, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, loc)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, or lacks an employer (which would break the
// company slug). The employer comes from the posting, not the board file: a Compleo tenant may
// be a recruiter posting on behalf of the hiring company.
func (s compleo) detail(ctx context.Context, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p compleoPosting
	if !ldJobPosting(root, &p) || p.HiringOrganization.Name == "" {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()

	return Job{
		ExternalID:  compleoJobID(loc),
		URL:         loc,
		Title:       strings.TrimSpace(p.Title),
		Company:     p.HiringOrganization.Name,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location) || isRemote(p.Title),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// compleoPosting is the schema.org JobPosting decoded from a Compleo job page. jobLocation is a
// single Place (not an array), and datePosted is RFC3339.
type compleoPosting struct {
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	DatePosted         string      `json:"datePosted"`
	JobLocation        schemaPlace `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// compleoJobIDPattern captures the native posting code from a /<tenant>/jobdetail/<code> URL.
var compleoJobIDPattern = regexp.MustCompile(`/jobdetail/([A-Za-z0-9]+)/?$`)

// compleoJobID extracts the native posting id from a job page URL, or "" when the URL is not a
// job posting (so the platform's other pages are dropped from the sitemap).
func compleoJobID(loc string) string {
	return firstSubmatch(compleoJobIDPattern, loc)
}
