package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// radancy adapts Radancy (TalentBrew) career sites. The board is the career-site host
// (e.g. "careers.ing.com"). The site's sitemap.xml enumerates the pages; each job page is
// server-rendered HTML carrying a schema.org JobPosting ld+json block, so the description
// comes from a per-job detail fetch (bounded-concurrency), like the other detail adapters
// (successfactors, icims).

// radancyHTTP is the transport radancy needs: an XML sitemap plus HTML detail pages.
type radancyHTTP interface {
	XMLGetter
	HTMLGetter
}

type radancy struct {
	http radancyHTTP
}

// NewRadancy builds the Radancy adapter over the given HTTP client.
func NewRadancy(c radancyHTTP) Source { return radancy{http: c} }

func (radancy) Provider() string { return "radancy" }

// radancySitemapEntry is one <url> of the sitemap: the page URL (a job page, a category
// page, or the landing page).
type radancySitemapEntry struct {
	Loc string `xml:"loc"`
}

func (s radancy) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []radancySitemapEntry `xml:"url"`
	}
	url := fmt.Sprintf("https://%s/sitemap.xml", e.Board)
	if err := s.http.GetXML(ctx, url, &sitemap); err != nil {
		return nil, fmt.Errorf("radancy: sitemap %s: %w", e.Board, err)
	}

	// Keep only real job postings: a loc with a /job/ segment and a trailing numeric id.
	// This drops the /category/ and landing entries, which carry no posting id.
	var locs []string
	for _, u := range sitemap.URLs {
		if radancyJobID(u.Loc) != "" {
			locs = append(locs, u.Loc)
		}
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails or carries no JobPosting, so the caller skips just that posting.
func (s radancy) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p radancyPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := ""
	if len(p.JobLocation) > 0 {
		a := p.JobLocation[0].Address
		location = joinNonEmpty(a.AddressLocality, radancyRegion(a.AddressRegion), a.AddressCountry)
	}

	// jobLocationType is the authoritative remote signal when present; isRemote is only a
	// fallback. WorkMode carries the structured signal alone, set only from TELECOMMUTE.
	remote := p.JobLocationType == "TELECOMMUTE"

	return Job{
		ExternalID:  radancyJobID(loc),
		URL:         loc,
		Title:       p.Title,
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      remote || isRemote(location) || isRemote(p.Title),
		WorkMode:    workModeFromRemote(remote),
		// datePosted is non-zero-padded ("2026-6-17"), so it needs the flexible layout.
		PostedAt: parseLayout("2006-1-2", p.DatePosted),
	}, true
}

// radancyPosting is the schema.org JobPosting decoded from a Radancy job page's
// application/ld+json block.
type radancyPosting struct {
	Title              string         `json:"title"`
	Description        string         `json:"description"`
	DatePosted         string         `json:"datePosted"`
	JobLocationType    string         `json:"jobLocationType"`
	JobLocation        []radancyPlace `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// radancyPlace is one entry of JobPosting.jobLocation (Radancy emits an array).
type radancyPlace struct {
	Address struct {
		AddressLocality string `json:"addressLocality"`
		AddressRegion   string `json:"addressRegion"`
		AddressCountry  string `json:"addressCountry"`
	} `json:"address"`
}

// radancyNumericRegion matches an addressRegion that is just a numeric code (some Radancy
// tenants emit e.g. "10" instead of a region name); such codes are noise in a location.
var radancyNumericRegion = regexp.MustCompile(`^\d+$`)

// radancyRegion blanks a purely numeric region code so it never leaks into a composed
// location, returning the value unchanged otherwise.
func radancyRegion(s string) string {
	if radancyNumericRegion.MatchString(s) {
		return ""
	}
	return s
}

// radancyJobIDPattern captures the trailing numeric posting id of a job URL (the last path
// segment, e.g. ".../3121/39724266176" → "39724266176"). The /job/ guard plus this trailing
// id together exclude the /category/ and landing entries.
var radancyJobIDPattern = regexp.MustCompile(`/job/.*/(\d+)/?$`)

// radancyJobID extracts the native numeric posting id from a job page URL, or "" when the
// URL is not a job posting.
func radancyJobID(loc string) string {
	return firstSubmatch(radancyJobIDPattern, loc)
}
