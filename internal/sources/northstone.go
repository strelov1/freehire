package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// northstone adapts the Northstone Holdings recruiting-brand career sites (e.g. Revun,
// Langford Staffing, "Vetted by EnzRossi"). The brands share one server-rendered Next.js
// template: a listing at https://<host>/<section>/ (the trailing slash is required — without
// it the site 308-redirects) whose anchors link to /<section>/<slug> detail pages, each
// carrying a schema.org JobPosting ld+json. The board encodes both parts as "<host>/<section>"
// (e.g. "www.revun.com/careers", "vetted.enzrossi.com/positions"); the native posting id is the
// JobPosting identifier.value (a per-brand backend id — a UUID for EnzRossi, a Zoho Recruit id
// for Revun/Langford), which is stable across the site's slug changes. Description and structured
// fields come from a per-posting detail fetch (bounded-concurrency), like the other JSON-LD
// detail adapters (careerspage/epam).
type northstone struct {
	http HTMLGetter
}

// NewNorthstone builds the Northstone recruiting-brand adapter over the given HTML client.
func NewNorthstone(c HTMLGetter) Source { return northstone{http: c} }

func (northstone) Provider() string { return "northstone" }

func (s northstone) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host, section, err := northstoneBoard(e.Board)
	if err != nil {
		return nil, err
	}
	// The trailing slash is load-bearing: the listing 308-redirects without it.
	base, err := url.Parse(fmt.Sprintf("https://%s/%s/", host, section))
	if err != nil {
		return nil, fmt.Errorf("northstone: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("northstone: listing %s: %w", e.Board, err)
	}

	// Each posting's fields come from its own detail fetch, fanned out under a bounded pool.
	locs := jobLinks(base, root, northstoneIsJob(section))
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one job's detail page and maps its JobPosting ld+json to a Job, returning
// ok=false when the fetch fails, the page carries no JobPosting, or the posting has no native
// id — so the caller skips just that posting.
func (s northstone) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	loc = northstoneCanonical(loc)
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p northstonePosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	id := strings.TrimSpace(p.Identifier.Value)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}

	location := p.location()
	// jobLocationType is the only structured work-arrangement signal: TELECOMMUTE means remote.
	// Absent → leave WorkMode empty and fall back to the location text for the remote flag.
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	workMode := ""
	if remote {
		workMode = "remote"
	}
	return Job{
		ExternalID:     id,
		URL:            loc,
		Title:          p.Title,
		Company:        firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:       location,
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         remote || isRemote(location),
		WorkMode:       workMode,
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       northstoneDate(p.DatePosted),
	}, true
}

// northstoneDate parses the JobPosting datePosted, which arrives as a full RFC3339 timestamp
// on some brands (EnzRossi) and as a bare date on others (Revun's "2026-07-01").
func northstoneDate(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	return parseDate(s)
}

// northstonePosting is the schema.org JobPosting decoded from a detail page's ld+json block.
// jobLocation is a single Place (null for a fully-remote posting) and applicantLocationRequirements
// an array of Country; location() prefers the former and falls back to the latter.
type northstonePosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressRegion   string `json:"addressRegion"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
	ApplicantLocationRequirements []struct {
		Name string `json:"name"`
	} `json:"applicantLocationRequirements"`
	Identifier struct {
		Value string `json:"value"`
	} `json:"identifier"`
}

// location builds the free-text location, preferring the explicit jobLocation address and
// falling back to the applicant-location countries (the only signal a fully-remote posting
// exposes, its jobLocation being null).
func (p northstonePosting) location() string {
	if loc := joinNonEmpty(
		p.JobLocation.Address.AddressLocality,
		p.JobLocation.Address.AddressRegion,
		p.JobLocation.Address.AddressCountry,
	); loc != "" {
		return loc
	}
	names := make([]string, 0, len(p.ApplicantLocationRequirements))
	for _, c := range p.ApplicantLocationRequirements {
		names = append(names, c.Name)
	}
	return joinNonEmpty(names...)
}

// northstoneBoard splits the "<host>/<section>" board into its host and single-segment
// listing path, rejecting a board that is missing either part or carries a deeper path.
func northstoneBoard(board string) (host, section string, err error) {
	board = strings.Trim(strings.TrimSpace(board), "/")
	host, section, ok := strings.Cut(board, "/")
	if !ok || host == "" || section == "" || strings.Contains(section, "/") {
		return "", "", fmt.Errorf("northstone: board %q must be %q", board, "<host>/<section>")
	}
	return host, section, nil
}

// northstoneIsJob returns the job-link predicate for a listing under /<section>/: it accepts a
// single-segment /<section>/<slug> detail anchor and rejects the bare listing, other sections,
// and any deeper sub-path. A non-job link that slips through (e.g. /careers/faq) just fails the
// JobPosting decode and is dropped, so the predicate need not be exhaustive.
func northstoneIsJob(section string) func(string) bool {
	re := regexp.MustCompile(`^/` + regexp.QuoteMeta(section) + `/[^/?#]+/?$`)
	return func(href string) bool {
		u, err := url.Parse(href)
		if err != nil {
			return false
		}
		return re.MatchString(u.Path)
	}
}

// northstoneCanonical ensures the detail URL ends in the trailing slash the sites redirect to,
// so we fetch (and store) the canonical form directly rather than through a 308.
func northstoneCanonical(loc string) string {
	u, err := url.Parse(loc)
	if err != nil || u.Path == "" || strings.HasSuffix(u.Path, "/") {
		return loc
	}
	u.Path += "/"
	return u.String()
}
