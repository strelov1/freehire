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
		Jobs []struct {
			ID              string `json:"id"`
			Title           string `json:"title"`
			Location        string `json:"location"`
			JobURL          string `json:"jobUrl"`
			PublishedAt     string `json:"publishedAt"`
			DescriptionHTML string `json:"descriptionHtml"`
			IsRemote        bool   `json:"isRemote"`
			EmploymentType  string `json:"employmentType"`
		} `json:"jobs"`
	}
	if err := a.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("ashby: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		jobs = append(jobs, Job{
			ExternalID:     j.ID,
			URL:            j.JobURL,
			Title:          j.Title,
			Company:        e.Company,
			Location:       j.Location,
			Description:    sanitizeHTML(j.DescriptionHTML),
			Remote:         j.IsRemote,
			WorkMode:       workModeFromRemote(j.IsRemote),
			PostedAt:       parseRFC3339(j.PublishedAt),
			EmploymentType: ashbyEmploymentType(j.EmploymentType),
		})
	}
	return jobs, nil
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
