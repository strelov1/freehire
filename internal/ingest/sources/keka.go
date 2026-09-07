package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// keka adapts Keka career portals, an Indian HR/ATS SaaS. The board is the company's Keka
// subdomain (e.g. "minfy" for minfy.keka.com/careers). The plain /careers/ bootstrap page
// carries no jobs itself — it fetches a static content fragment keyed by the org's own GUID,
// embedded in that fragment's own request URL — but once the GUID is known, one request to
// the platform's embedded-jobs widget API (the same one an org embeds on its own site) returns
// every open posting fully detailed, description included, so no per-posting detail fetch is
// needed at all.
type kekaHTTP interface {
	TextGetter
	JSONGetter
}

type keka struct {
	http kekaHTTP
}

// NewKeka builds the Keka adapter over the given HTTP client.
func NewKeka(c kekaHTTP) Source { return keka{http: c} }

func (keka) Provider() string { return "keka" }

// fullBoardListing: the embedded-jobs API returns the board's whole open-postings array in one
// request (verified live against an 88-posting board, no pagination), so a listing failure
// aborts the whole Fetch rather than silently returning a partial board.
func (keka) fullBoardListing() {}

func (s keka) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base := fmt.Sprintf("https://%s.keka.com/careers", e.Board)

	home, err := s.http.GetText(ctx, base+"/")
	if err != nil {
		return nil, fmt.Errorf("keka: board home %s: %w", e.Board, err)
	}
	orgID := kekaOrgID(home)
	if orgID == "" {
		return nil, fmt.Errorf("keka: board %s: no org id on the careers page", e.Board)
	}

	// The org's own display name is a separate, best-effort lookup: a failure here still
	// leaves the board's configured name to fall back on, so it does not abort the crawl the
	// way a failed job listing does.
	company := e.Company
	var info struct {
		Name string `json:"name"`
	}
	if err := s.http.GetJSON(ctx, base+"/api/organization/default/careerportalinfo", &info); err == nil && info.Name != "" {
		company = info.Name
	}

	var postings []kekaPosting
	if err := s.http.GetJSON(ctx, fmt.Sprintf("%s/api/embedjobs/default/active/%s", base, orgID), &postings); err != nil {
		return nil, fmt.Errorf("keka: list board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, kekaJob(base, company, p))
	}
	return jobs, nil
}

// kekaJobTypeFullTime is the only jobType value the platform's own embed widget renders as
// "Full Time" (job.jobType == 2 ? "Full Time" : "Part time", read out of its widget script);
// any other value is a part-time posting, so a plain zero-value default cannot stand in for it.
const kekaJobTypeFullTime = 2

func kekaJob(base, company string, p kekaPosting) Job {
	location := ""
	if len(p.JobLocations) > 0 {
		l := p.JobLocations[0]
		location = joinNonEmpty(l.City, l.State, l.CountryName)
	}
	employmentType := "part_time"
	if p.JobType == kekaJobTypeFullTime {
		employmentType = "full_time"
	}

	return Job{
		ExternalID:  strconv.Itoa(p.ID),
		URL:         fmt.Sprintf("%s/jobdetails/%d", base, p.ID),
		Title:       p.Title,
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		// Keka carries no structured remote flag in the fields this board's org configured
		// (jobListingSetting.jobFields lists only location/experience/jobType/dateOfPosting);
		// the location/title heuristic is the only signal available.
		Remote:         isRemote(location) || isRemote(p.Title),
		EmploymentType: employmentType,
		PostedAt:       parseRFC3339(p.PublishedOn),
	}
}

// kekaPosting is one entry from the embedded-jobs "active" listing — already the full
// posting, description included, so no separate detail fetch is needed.
type kekaPosting struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	JobLocations []struct {
		City        string `json:"city"`
		State       string `json:"state"`
		CountryName string `json:"countryName"`
	} `json:"jobLocations"`
	JobType     int    `json:"jobType"`
	PublishedOn string `json:"publishedOn"`
}

// kekaOrgIDPattern captures the org's GUID from the bootstrap page's embedded fragment
// fetch, e.g. fetch('/ats/documents/41f734d9-0db2-420a-b416-461b07fc97ac/careerportal/...').
var kekaOrgIDPattern = regexp.MustCompile(`/ats/documents/([0-9a-fA-F-]{36})/careerportal/`)

// kekaOrgID extracts the org's GUID from the board's bootstrap page, or "" when the page
// carries none (an unrecognized shape — the board's identity cannot be resolved).
func kekaOrgID(home string) string {
	return firstSubmatch(kekaOrgIDPattern, home)
}
