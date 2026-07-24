package sources

import (
	"context"
	"fmt"
)

// ashbyBaseURL is the Ashby public job-board API root.
const ashbyBaseURL = "https://api.ashbyhq.com/posting-api/job-board"

// ashby adapts the Ashby public job-board API. The list endpoint carries an HTML
// description and an explicit remote flag, so no per-posting detail request is needed.
type ashby struct {
	http JSONGetter
}

// NewAshby builds the Ashby adapter over the given HTTP client.
func NewAshby(c JSONGetter) Source { return ashby{http: c} }

func (ashby) Provider() string { return "ashby" }

func (a ashby) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("%s/%s", ashbyBaseURL, e.Board)

	var resp struct {
		Jobs []AshbyPosting `json:"jobs"`
	}
	if err := a.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("ashby: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		job := MapAshbyPosting(j)
		job.ExternalID = j.ID
		job.Company = e.Company
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// AshbyPosting is one job from the Ashby public job-board API, exported so the
// link-following adapter (internal/linksource) decodes the same payload.
type AshbyPosting struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Location        string `json:"location"`
	JobURL          string `json:"jobUrl"`
	PublishedAt     string `json:"publishedAt"`
	DescriptionHTML string `json:"descriptionHtml"`
	IsRemote        bool   `json:"isRemote"`
	EmploymentType  string `json:"employmentType"`
}

// MapAshbyPosting maps an Ashby API posting into a Job, so the board adapter and the
// link-following adapter produce identical facets for the same posting. Remote unions the
// explicit isRemote flag with the location heuristic (a false flag with a "Remote"
// location is genuinely remote — the greenhouse convention), while WorkMode carries the
// structured signal only. ExternalID and Company are left to the caller: the board
// adapter sets the raw id (the pipeline namespaces it by board) and the configured
// company; the link resolver namespaces the id itself and derives the company from the
// board slug (the per-board API carries no company name).
func MapAshbyPosting(j AshbyPosting) Job {
	return Job{
		URL:            j.JobURL,
		Title:          j.Title,
		Location:       j.Location,
		Description:    sanitizeHTML(j.DescriptionHTML),
		Remote:         j.IsRemote || isRemote(j.Location),
		WorkMode:       workModeFromRemote(j.IsRemote),
		PostedAt:       parseRFC3339(j.PublishedAt),
		EmploymentType: ashbyEmploymentType(j.EmploymentType),
	}
}

// ashbyEmploymentType maps Ashby's employmentType enum onto the freehire vocabulary,
// returning "" for an unknown/absent value so the description parser decides.
func ashbyEmploymentType(t string) string {
	switch t {
	case "FullTime":
		return "full_time"
	case "PartTime":
		return "part_time"
	case "Contract", "Temporary":
		return "contract"
	case "Intern", "Internship":
		return "internship"
	default:
		return ""
	}
}
