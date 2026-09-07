package sources

import (
	"context"
	"fmt"
	"strconv"
)

// keka adapts Keka career portals, an Indian HR/ATS SaaS. The board is the company's Keka
// subdomain (e.g. "minfy" for minfy.keka.com/careers). One request to the platform's own
// job-listing API (the same one its careers page's own frontend calls, whichever of its two
// generations a given org's portal renders with — jobListingFormat 1 or 2, verified live
// against one of each) returns every open posting fully detailed, description included, so no
// per-posting detail fetch is needed at all.
type kekaHTTP interface {
	JSONGetter
}

type keka struct {
	http kekaHTTP
}

// NewKeka builds the Keka adapter over the given HTTP client.
func NewKeka(c kekaHTTP) Source { return keka{http: c} }

func (keka) Provider() string { return "keka" }

// fullBoardListing: the jobs-active endpoint returns the board's whole open-postings array in
// one request (verified live against an 88-posting board, no pagination), so a listing failure
// aborts the whole Fetch rather than silently returning a partial board.
func (keka) fullBoardListing() {}

func (s keka) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base := fmt.Sprintf("https://%s.keka.com/careers", e.Board)

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
	if err := s.http.GetJSON(ctx, base+"/api/jobs/default/active", &postings); err != nil {
		return nil, fmt.Errorf("keka: list board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, kekaJob(base, company, p))
	}
	return jobs, nil
}

// kekaJobTypeFullTime is the only jobType value the platform's own frontend renders as
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
		// Keka carries no structured remote flag in the fields either template's org
		// configures (jobListingSetting.jobFields lists only some mix of
		// location/experience/jobType/dateOfPosting); the location/title heuristic is the
		// only signal available.
		Remote:         isRemote(location) || isRemote(p.Title),
		EmploymentType: employmentType,
		PostedAt:       parseRFC3339(p.PublishedOn),
	}
}

// kekaPosting is one entry from the jobs-active listing — already the full posting,
// description included, so no separate detail fetch is needed.
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
