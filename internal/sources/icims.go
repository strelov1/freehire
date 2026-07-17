package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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
	host := icimsHost(e.Board)
	locs, err := s.jobLocs(ctx, host, e.Board)
	if err != nil {
		return nil, err
	}

	// Each job's posting comes from its own iframe-fragment fetch, fanned out under a
	// bounded pool. vanity boards (careers-home) build the fragment URL differently.
	vanity := icimsVanity(e.Board)
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, host, vanity, loc)
	}), nil
}

// icimsVanity reports whether the board is a full vanity host (contains a dot) rather than a
// bare iCIMS slug. A vanity site (e.g. careers.docusign.com) runs the newer "careers-home"
// product: its sitemap is an index and its job detail lives at /careers-home/jobs/<id>.
func icimsVanity(board string) bool { return strings.Contains(board, ".") }

// icimsHost resolves the board to the host the endpoints use: a vanity board is itself the
// host; a bare slug forms the classic "careers-<slug>.icims.com".
func icimsHost(board string) string {
	if icimsVanity(board) {
		return board
	}
	return "careers-" + board + ".icims.com"
}

// icimsSitemap decodes either sitemap shape: a flat <urlset> (child <url>) or a
// <sitemapindex> (child <sitemap>). Both nest a <loc>, so one entry type serves both.
type icimsSitemap struct {
	URLs     []icimsSitemapEntry `xml:"url"`
	Sitemaps []icimsSitemapEntry `xml:"sitemap"`
}

// jobLocs collects the board's job-posting URLs from its sitemap, following a sitemap index
// one level deep (vanity/careers-home sites nest their <url> entries under sub-sitemaps) and
// keeping only locs with a parseable /jobs/<id> segment — dropping the /jobs/search and
// /jobs/intro entries, which carry no numeric id.
func (s icims) jobLocs(ctx context.Context, host, board string) ([]string, error) {
	var root icimsSitemap
	if err := s.http.GetXML(ctx, fmt.Sprintf("https://%s/sitemap.xml", host), &root); err != nil {
		return nil, fmt.Errorf("icims: sitemap %s: %w", board, err)
	}
	entries := root.URLs
	for _, sm := range root.Sitemaps {
		var sub icimsSitemap
		if err := s.http.GetXML(ctx, sm.Loc, &sub); err != nil {
			// Skip a flaky sub-sitemap rather than losing the whole board — the same
			// per-entry isolation the detail fan-out uses; the missed postings reappear
			// on the next crawl. Only a failed ROOT sitemap fails the board.
			continue
		}
		entries = append(entries, sub.URLs...)
	}
	var locs []string
	for _, u := range entries {
		if icimsJobID(u.Loc) != "" {
			locs = append(locs, u.Loc)
		}
	}
	return locs, nil
}

// detail fetches one job's "?in_iframe=1" fragment and maps its JobPosting ld+json to a
// Job, returning ok=false when the fragment fetch fails or carries no JobPosting, so the
// caller skips just that posting.
func (s icims) detail(ctx context.Context, e CompanyEntry, host string, vanity bool, loc string) (Job, bool) {
	id := icimsJobID(loc)
	// Classic hosts serve the fragment at <loc>?in_iframe=1. Vanity/careers-home locs are
	// /jobs/<id>?lang=… (a query already), so their fragment lives at a distinct path.
	frag := loc + "?in_iframe=1"
	if vanity {
		frag = fmt.Sprintf("https://%s/careers-home/jobs/%s?in_iframe=1", host, id)
	}
	root, err := s.http.GetHTML(ctx, frag)
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
		ExternalID:  id,
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
	JobLocation        schemaPlaces `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// icimsJobIDPattern captures the numeric posting id from a job URL's /jobs/<id> segment,
// terminated by a slash, query, fragment, or end of string. This matches both the classic
// host form (/jobs/<id>/<slug>/job) and the vanity/careers-home form (/jobs/<id>?lang=…),
// while the non-posting /jobs/search and /jobs/intro entries (no digits) yield no match.
var icimsJobIDPattern = regexp.MustCompile(`/jobs/(\d+)(?:[/?#]|$)`)

// icimsJobID extracts the native numeric posting id from a job page URL, or "" when the
// URL is not a job posting.
func icimsJobID(loc string) string {
	return firstSubmatch(icimsJobIDPattern, loc)
}

// icimsAvailable blanks the iCIMS "UNAVAILABLE" placeholder so it never leaks into a
// composed location, returning the value unchanged otherwise.
func icimsAvailable(s string) string {
	if s == "UNAVAILABLE" {
		return ""
	}
	return s
}
