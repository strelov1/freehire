package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// crelate adapts Crelate candidate portals through their keyless public API. Crelate is a
// recruiting/staffing ATS; a portal at jobs.crelate.com/portal/<slug> commonly fronts a network of
// client companies, so this is an aggregator — each posting's employer comes from its CompanyName.
//
// The board is "<portalSlug>:<organizationId>": the OrganizationId GUID (public, embedded in the
// portal page) keys the API, and the portal slug builds each posting's human job URL. One
// GetAllJobs call returns every published posting fully populated (structured location,
// LastPostedOnDate, an HTML/plain description), so no per-posting detail fetch or pagination is
// needed.
//
// Note: the ever-jobs recipe (app.crelate.com/api3/jobs with X-Api-Key=slug) is outdated and 401s;
// the working endpoint is this candidateportal GetAllJobs call, keyed only by the OrganizationId.
type crelate struct {
	http JSONGetter
}

// NewCrelate builds the Crelate adapter over the given HTTP client.
func NewCrelate(c JSONGetter) Source { return crelate{http: c} }

func (crelate) Provider() string { return "crelate" }

// aggregator: a Crelate portal lists many client companies, so it stays in the source facet and
// each job's company comes from the posting, not the board.
func (crelate) aggregator() {}

// crelateResponse is the GetAllJobs envelope.
type crelateResponse struct {
	Jobs         []crelateJob `json:"Jobs"`
	IsError      bool         `json:"IsError"`
	ErrorMessage string       `json:"ErrorMessage"`
}

type crelateJob struct {
	ID               string `json:"Id"`
	Title            string `json:"Title"`
	CompanyName      string `json:"CompanyName"`
	Description      string `json:"Description"`
	City             string `json:"City"`
	State            string `json:"State"`
	Country          string `json:"Country"`
	URL              string `json:"Url"`
	JobCode          string `json:"JobCode"`
	LastPostedOnDate string `json:"LastPostedOnDate"`
}

func (s crelate) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	colon := strings.Index(e.Board, ":")
	if colon < 0 {
		return nil, fmt.Errorf("crelate: board %q must be %q", e.Board, "<portalSlug>:<organizationId>")
	}
	slug, org := e.Board[:colon], e.Board[colon+1:]
	if slug == "" || org == "" {
		return nil, fmt.Errorf("crelate: board %q must be %q with both parts set", e.Board, "<portalSlug>:<organizationId>")
	}

	envelope := fmt.Sprintf(`{"Locations":null,"OrganizationId":%q,"SearchText":null,"Tags":null}`, org)
	q := url.Values{}
	q.Set("requestEnvelope", envelope)
	reqURL := "https://jobs.crelate.com/api/candidateportal/GetAllJobs?" + q.Encode()

	var resp crelateResponse
	if err := s.http.GetJSON(ctx, reqURL, &resp); err != nil {
		return nil, fmt.Errorf("crelate: board %s: %w", e.Board, err)
	}
	if resp.IsError {
		return nil, fmt.Errorf("crelate: board %s: portal error: %s", e.Board, resp.ErrorMessage)
	}

	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		if job, ok := toCrelateJob(j, slug, e); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toCrelateJob maps a posting to a Job, returning ok=false for a posting missing an id (which
// would collide on the dedup key). The employer is the posting's CompanyName, falling back to the
// configured company.
func toCrelateJob(j crelateJob, slug string, e CompanyEntry) (Job, bool) {
	if strings.TrimSpace(j.ID) == "" {
		return Job{}, false
	}
	company := strings.TrimSpace(j.CompanyName)
	if company == "" {
		company = e.Company
	}
	return Job{
		ExternalID:  j.ID,
		URL:         crelateJobURL(slug, j),
		Title:       j.Title,
		Company:     company,
		Location:    crelateLocation(j),
		Description: sanitizeHTML(j.Description),
		PostedAt:    parseRFC3339(j.LastPostedOnDate),
	}, true
}

// crelateJobURL builds the human portal job page from the slug and job code (the API's Url is a
// relative "/<JobCode>").
func crelateJobURL(slug string, j crelateJob) string {
	code := j.JobCode
	if code == "" {
		code = strings.TrimPrefix(j.URL, "/")
	}
	return fmt.Sprintf("https://jobs.crelate.com/portal/%s/job/%s", slug, code)
}

// crelateLocation renders "City, State", falling back to the country.
func crelateLocation(j crelateJob) string {
	var parts []string
	for _, p := range []string{j.City, j.State} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	return strings.TrimSpace(j.Country)
}
