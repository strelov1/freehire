package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// techtree adapts jobs.techtree.dev, a small AI-recruiting-agency job board. It is boardless
// (one site, no per-tenant board) and multi-company, so it stays in the source facet and
// takes each posting's company from the page. The flat sitemap.xml enumerates every open
// posting's URL; each job page server-renders a schema.org JobPosting ld+json block plus,
// separately, the full rich-text body in a "prose"-class DOM container — the ld+json block's
// own description field is only the page's short summary, not the full body.

// techtreeHTTP is the transport techtree needs: an XML sitemap plus HTML detail pages.
type techtreeHTTP interface {
	XMLGetter
	HTMLGetter
}

type techtree struct {
	http techtreeHTTP
}

// NewTechTree builds the TechTree adapter over the given HTTP client.
func NewTechTree(c techtreeHTTP) Source { return techtree{http: c} }

func (techtree) Provider() string { return "techtree" }

func (techtree) boardless() {}

func (techtree) aggregator() {}

const techtreeSitemapURL = "https://jobs.techtree.dev/sitemap.xml"

// techtreeJobIDPattern captures the job UUID from a /job/<uuid> path, ignoring any trailing
// query string (e.g. the site's own ?tp=<tracking-id> parameter).
var techtreeJobIDPattern = regexp.MustCompile(`/job/([0-9a-fA-F-]{36})`)

// techtreeJobID extracts the native posting id from a job page URL, or "" when the URL is not
// a job posting (so non-job sitemap entries are dropped).
func techtreeJobID(loc string) string {
	return firstSubmatch(techtreeJobIDPattern, loc)
}

func (s techtree) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	locs, err := sitemapJobLocs(ctx, s.http, techtreeSitemapURL, techtreeJobID)
	if err != nil {
		return nil, fmt.Errorf("techtree: sitemap: %w", err)
	}

	jobs := fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, loc)
	})

	// A run that listed postings and read none of them is a board failure, not an empty
	// crawl — see internal/ingest/sources/AGENTS.md's "a crawl that reads nothing of what it
	// listed must fail" (the echojobs#2588 lesson).
	if len(locs) > 0 && len(jobs) == 0 {
		return nil, fmt.Errorf("techtree: listed %d postings and read none of them", len(locs))
	}
	return jobs, nil
}

// detail fetches one job page and maps its ld+json JobPosting plus its DOM rich-text body to
// a Job, returning ok=false when the page fetch fails, carries no JobPosting, has no
// resolvable employer, or has no locatable full-text body — so the caller drops just that
// posting.
func (s techtree) detail(ctx context.Context, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p techtreePosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	company := strings.TrimSpace(p.HiringOrganization.Name)
	if company == "" {
		return Job{}, false
	}

	body := firstByClass(root, "prose")
	if body == nil {
		return Job{}, false
	}
	description := sanitizeHTML(innerHTML(body))

	location := strings.TrimSpace(p.JobLocation.Address)

	return Job{
		ExternalID:     techtreeJobID(loc),
		URL:            loc,
		Title:          strings.TrimSpace(p.Title),
		Company:        company,
		Location:       location,
		Description:    description,
		Remote:         isRemote(location),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       techtreePostedAt(p.DatePosted),
	}, true
}

// techtreePostedAt parses TechTree's own datePosted shape ("2026-09-11T07:46:06.765320" —
// a zone-less ISO-8601 timestamp, confirmed live to carry no offset or "Z"), which
// parseRFC3339 does not accept since both its layouts require one. A zone-less layout has
// Go default to UTC, the reasonable read for a backend timestamp with no stated zone.
// parseRFC3339 is tried first so a future switch to a proper RFC3339 value keeps working
// unchanged.
func techtreePostedAt(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	return parseLayout("2006-01-02T15:04:05.999999", s)
}

// techtreePosting is the schema.org JobPosting decoded from a TechTree job page.
// JobLocation.Address is a plain string on this platform ("Latin America (remote)"), not a
// structured PostalAddress object, so it cannot reuse the shared schemaAddress/schemaPlace
// types. Description is deliberately NOT read here — TechTree's own ld+json description
// field is only the page's short summary, never the full body (see detail, which reads the
// full body from the DOM instead).
type techtreePosting struct {
	Title              string `json:"title"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address string `json:"address"`
	} `json:"jobLocation"`
}
