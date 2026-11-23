package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"

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

	var failed atomic.Pointer[error]
	jobs := fetchDetails(mine, defaultDetailWorkers, func(loc string) (Job, bool) {
		job, found, err := s.FetchPosting(ctx, loc)
		if err != nil {
			failed.CompareAndSwap(nil, &err)
		}
		return job, found
	})
	// Partial reads stay a success; a board whose listed postings all failed to read must not
	// look like an empty crawl. A listed posting that is merely gone (404/410) is not a failure.
	if len(jobs) == 0 && failed.Load() != nil {
		return nil, *failed.Load()
	}
	return jobs, nil
}

// FetchPosting reads a Compleo detail URL from the sitemap or the board recognizer.
// The employer on the posting may differ from the recruiting agency operating the board.
func (s compleo) FetchPosting(ctx context.Context, loc string) (Job, bool, error) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		var status *StatusError
		if errors.As(err, &status) && (status.Code == http.StatusNotFound || status.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, fmt.Errorf("compleo: posting %s: %w", compleoJobID(loc), err)
	}
	var p compleoPosting
	if !ldJobPosting(root, &p) || p.HiringOrganization.Name == "" {
		return Job{}, false, nil
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
	}, true, nil
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
