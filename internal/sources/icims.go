package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// icims adapts iCIMS career sites. The board is the iCIMS slug (e.g. "360care"), forming
// the host "careers-{board}.icims.com". The site's sitemap.xml enumerates the postings;
// each canonical job page is a SPA/wrapper, but its embedded "?in_iframe=1" fragment is
// server-rendered and carries a schema.org JobPosting ld+json block, so the description
// comes from a per-job detail fetch (bounded-concurrency), like the other detail adapters.

// icimsHTTP is the transport iCIMS needs: an XML sitemap plus HTML detail fragments.
type icimsHTTP interface {
	XMLGetter
	HTMLGetter
}

type icims struct {
	http icimsHTTP
}

// NewICIMS builds the iCIMS adapter over the given HTTP client.
func NewICIMS(c icimsHTTP) Source { return icims{http: c} }

func (icims) Provider() string { return "icims" }

// icimsSitemapEntry is one <url> of the sitemap: the page URL (a job page, the search
// page, or the intro page).
type icimsSitemapEntry struct {
	Loc string `xml:"loc"`
}

func (s icims) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []icimsSitemapEntry `xml:"url"`
	}
	url := fmt.Sprintf("https://careers-%s.icims.com/sitemap.xml", e.Board)
	if err := s.http.GetXML(ctx, url, &sitemap); err != nil {
		return nil, fmt.Errorf("icims: sitemap %s: %w", e.Board, err)
	}

	// Keep only real job postings: a loc with a parseable /jobs/<id>/ segment. This drops
	// the non-posting /jobs/search and /jobs/intro entries, which carry no numeric id.
	var locs []string
	for _, u := range sitemap.URLs {
		if icimsJobID(u.Loc) != "" {
			locs = append(locs, u.Loc)
		}
	}

	// Each job's posting comes from its own iframe-fragment fetch, fanned out under a
	// bounded pool.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one job's "?in_iframe=1" fragment and maps its JobPosting ld+json to a
// Job, returning ok=false when the fragment fetch fails or carries no JobPosting, so the
// caller skips just that posting.
func (s icims) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc+"?in_iframe=1")
	if err != nil {
		return Job{}, false
	}
	var p icimsPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := ""
	if len(p.JobLocation) > 0 {
		a := p.JobLocation[0].Address
		location = joinNonEmpty(
			icimsAvailable(a.AddressLocality),
			icimsAvailable(a.AddressRegion),
			icimsAvailable(a.AddressCountry),
		)
	}

	// jobLocationType is the authoritative remote signal; isRemote(location) is only a
	// fallback. WorkMode carries the structured signal alone, so it is set only from
	// TELECOMMUTE, never the location heuristic.
	remote := p.JobLocationType == "TELECOMMUTE"

	return Job{
		ExternalID:  icimsJobID(loc),
		URL:         loc,
		Title:       p.Title,
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      remote || isRemote(location),
		WorkMode:    workModeFromRemote(remote),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// icimsPosting is the schema.org JobPosting decoded from an iCIMS job fragment's
// application/ld+json block.
type icimsPosting struct {
	Title              string       `json:"title"`
	Description        string       `json:"description"`
	DatePosted         string       `json:"datePosted"`
	JobLocationType    string       `json:"jobLocationType"`
	JobLocation        []icimsPlace `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// icimsPlace is one entry of JobPosting.jobLocation (iCIMS emits an array).
type icimsPlace struct {
	Address struct {
		AddressLocality string `json:"addressLocality"`
		AddressRegion   string `json:"addressRegion"`
		AddressCountry  string `json:"addressCountry"`
	} `json:"address"`
}

// icimsJobIDPattern captures the numeric posting id from a job URL's /jobs/<id>/ segment.
// The trailing slash is required, so the non-posting /jobs/search and /jobs/intro entries
// (no id, no trailing-slash digits) yield no match.
var icimsJobIDPattern = regexp.MustCompile(`/jobs/(\d+)/`)

// icimsJobID extracts the native numeric posting id from a job page URL, or "" when the
// URL is not a job posting.
func icimsJobID(loc string) string {
	if m := icimsJobIDPattern.FindStringSubmatch(loc); m != nil {
		return m[1]
	}
	return ""
}

// icimsAvailable blanks the iCIMS "UNAVAILABLE" placeholder so it never leaks into a
// composed location, returning the value unchanged otherwise.
func icimsAvailable(s string) string {
	if s == "UNAVAILABLE" {
		return ""
	}
	return s
}
