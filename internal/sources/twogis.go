package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// twogis adapts the 2GIS careers site (job.2gis.ru), a single-company source with no per-tenant
// board id (boardless). 2GIS runs a custom Next.js careers SPA fronting no supported ATS, but its
// /vacancies listing server-renders a /vacancies/<category>/<id> anchor for every posting and each
// vacancy page carries a schema.org JobPosting ld+json, so this adapter is the jobvite/dataart
// shape — listing to enumerate, per-vacancy detail fetch for the posting — over the shared ld+json
// helper.
type twogis struct {
	http HTMLGetter
}

const twogisBaseURL = "https://job.2gis.ru"

// NewTwoGIS builds the 2GIS adapter over the given HTTP client.
func NewTwoGIS(c HTMLGetter) Source { return twogis{http: c} }

func (twogis) Provider() string { return "2gis" }

// twogis is single-company, so its config entries carry no board.
func (twogis) boardless() {}

func (t twogis) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(twogisBaseURL + "/")
	if err != nil {
		return nil, fmt.Errorf("2gis: base url: %w", err)
	}
	root, err := t.http.GetHTML(ctx, twogisBaseURL+"/vacancies")
	if err != nil {
		return nil, fmt.Errorf("2gis: listing: %w", err)
	}

	locs := jobLinks(base, root, func(href string) bool { return twogisJobID(href) != "" })

	// Each posting's fields come from its own detail fetch, fanned out under a bounded pool.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return t.detail(ctx, e, loc)
	}), nil
}

// detail fetches one vacancy page and maps its JobPosting ld+json to a Job, returning ok=false when
// the URL has no id, the fetch fails, or the page carries no JobPosting, so the caller skips just
// that posting.
func (t twogis) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := twogisJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := t.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p twogisPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	location := p.location()
	// jobLocationType TELECOMMUTE is 2GIS's structured remote signal, so it sets WorkMode (clean
	// provenance); located roles leave WorkMode empty and the pipeline parses the location.
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	workMode := ""
	if remote {
		workMode = "remote"
	}
	return Job{
		ExternalID: id,
		URL:        jobURL,
		Title:      p.Title,
		// The configured company is canonical (this is 2GIS's own site); the JSON-LD
		// hiringOrganization is ignored so the board never mislabels its postings.
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      remote || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseDate(p.DatePosted),
	}, true
}

// twogisJobIDPattern captures the numeric posting id from a /vacancies/<category>/<id> path,
// terminated by a slash, query, fragment, or end of string. The /vacancies listing root and a
// bare /vacancies/<category> link carry no /<id> segment and yield no match.
var twogisJobIDPattern = regexp.MustCompile(`/vacancies/[a-z0-9_-]+/(\d+)(?:[/?#]|$)`)

// twogisJobID extracts the native posting id from a vacancy page URL, or "" when the URL is not a
// vacancy posting.
func twogisJobID(loc string) string {
	return firstSubmatch(twogisJobIDPattern, loc)
}

// twogisPosting is the schema.org JobPosting decoded from a 2GIS vacancy page's ld+json. It reuses
// schemaPlaces, the shared single-or-array jobLocation decoder (2GIS emits a single Place object for
// located roles and omits it for remote ones).
type twogisPosting struct {
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	DatePosted      string       `json:"datePosted"`
	JobLocationType string       `json:"jobLocationType"`
	JobLocation     schemaPlaces `json:"jobLocation"`
}

// location joins each jobLocation place as "City, Country" (falling back to whichever part is
// present), deduped and separated by "; ", so a job open in several cities lists them all.
func (p twogisPosting) location() string {
	var out []string
	seen := make(map[string]struct{})
	for _, pl := range p.JobLocation {
		s := joinNonEmpty(pl.Address.AddressLocality, pl.Address.AddressCountry)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return strings.Join(out, "; ")
}
