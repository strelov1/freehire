package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// jobvite adapts Jobvite career sites (jobs.jobvite.com/<company>). The board is the company's
// Jobvite careersite slug. The listing page server-renders a /<board>/job/<code> link for every
// posting; each job page carries a schema.org JobPosting ld+json, so the description comes from a
// per-posting detail fetch (bounded-concurrency) over the shared ld+json helper — the same shape
// as the careerspage/dataart adapters.
//
// The public careersite renders the full job list in one page (filtering is client-side, no
// server pagination), so Fetch reads every job anchor from the single listing. Per-page crawling
// is the seam to add here if a board ever exceeds Jobvite's server-render cap.
type jobvite struct {
	http HTMLGetter
}

// NewJobvite builds the Jobvite adapter over the given HTTP client.
func NewJobvite(c HTMLGetter) Source { return jobvite{http: c} }

func (jobvite) Provider() string { return "jobvite" }

func (j jobvite) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://jobs.jobvite.com/%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("jobvite: board %q: %w", e.Board, err)
	}
	root, err := j.http.GetHTML(ctx, base.String()+"jobs")
	if err != nil {
		return nil, fmt.Errorf("jobvite: listing %s: %w", e.Board, err)
	}

	locs := jobLinks(base, root, func(href string) bool { return jobviteJobID(href) != "" })

	// Each posting's fields come from its own detail fetch, fanned out under a bounded pool.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return j.detail(ctx, e, loc)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false when
// the URL has no code, the fetch fails, or the page carries no JobPosting, so the caller skips
// just that posting.
func (j jobvite) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := jobviteJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := j.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p jobvitePosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	location := p.location()
	return Job{
		ExternalID: id,
		URL:        jobURL,
		Title:      p.Title,
		// The configured company is canonical (the board is that employer's site); the JSON-LD
		// hiringOrganization is ignored so a board never mislabels its own postings.
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		// Jobvite states no structured jobLocationType, so remote is inferred from the location
		// text alone; WorkMode stays empty (the pipeline parses the location).
		Remote:   isRemote(location),
		PostedAt: parseDate(p.DatePosted),
	}, true
}

// jobviteJobIDPattern captures the alphanumeric posting code from a /<board>/job/<code> path,
// terminated by a slash, query, fragment, or end of string. The /<board>/jobs listing and the
// /<board>/jobAlerts nav link carry no /job/<code> segment and yield no match.
var jobviteJobIDPattern = regexp.MustCompile(`/job/([A-Za-z0-9]+)(?:[/?#]|$)`)

// jobviteJobID extracts the native posting code from a job page URL, or "" when the URL is not a
// job posting.
func jobviteJobID(loc string) string {
	return firstSubmatch(jobviteJobIDPattern, loc)
}

// jobvitePosting is the schema.org JobPosting decoded from a Jobvite job page's ld+json. It
// reuses schemaPlaces, the shared single-or-array jobLocation decoder (Jobvite emits an array, but
// the flexible form guards a board that emits a single Place object).
type jobvitePosting struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	DatePosted  string       `json:"datePosted"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// location joins each jobLocation place as "City, Country" (falling back to whichever part is
// present), deduped and separated by "; ", so a job open in several cities lists them all.
func (p jobvitePosting) location() string {
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
