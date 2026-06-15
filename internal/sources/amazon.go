package sources

import (
	"context"
	"fmt"
	"strings"
)

// amazon adapts Amazon's public careers search API (www.amazon.jobs/en/search.json), a
// single-company source with no per-tenant board id (boardless). The search endpoint
// returns each posting — including the full HTML description — inline, so one paged list
// call assembles every Job with no per-job detail fetch. An empty base_query returns the
// whole catalogue; pagination is by offset.
type amazon struct {
	http JSONGetter
}

const (
	amazonSearchURL = "https://www.amazon.jobs/en/search.json?result_limit=%d&offset=%d&sort=recent"
	amazonBaseURL   = "https://www.amazon.jobs"
	// amazonPageLimit is how many postings to request per page.
	amazonPageLimit = 100
	// amazonDateLayout matches the human posted_date Amazon emits ("June 15, 2026").
	amazonDateLayout = "January 2, 2006"
)

// NewAmazon builds the Amazon adapter over the given HTTP client.
func NewAmazon(c JSONGetter) Source { return amazon{http: c} }

func (amazon) Provider() string { return "amazon" }

// amazon is single-company, so its config entries carry no board.
func (amazon) boardless() {}

// amazonResponse is the search response. Hits is the catalogue size; Jobs is empty past
// the last page.
type amazonResponse struct {
	Hits int         `json:"hits"`
	Jobs []amazonJob `json:"jobs"`
}

// amazonJob is one posting from the search response. The description is HTML; id_icims
// arrives as a quoted string, not a number.
type amazonJob struct {
	IDICIMS            string `json:"id_icims"`
	Title              string `json:"title"`
	JobPath            string `json:"job_path"`
	NormalizedLocation string `json:"normalized_location"`
	Description        string `json:"description"`
	PostedDate         string `json:"posted_date"`
}

func (a amazon) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for offset := 0; ; offset += amazonPageLimit {
		var resp amazonResponse
		url := fmt.Sprintf(amazonSearchURL, amazonPageLimit, offset)
		if err := a.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("amazon: list offset %d: %w", offset, err)
		}
		if len(resp.Jobs) == 0 {
			break
		}
		for _, j := range resp.Jobs {
			jobs = append(jobs, a.toJob(e, j))
		}
		if len(jobs) >= resp.Hits {
			break
		}
	}
	return jobs, nil
}

// toJob maps a search result to a Job. The HTML description is sanitized; the job_path is
// resolved against the site origin.
func (amazon) toJob(e CompanyEntry, j amazonJob) Job {
	return Job{
		ExternalID:  j.IDICIMS,
		URL:         amazonBaseURL + j.JobPath,
		Title:       strings.TrimSpace(j.Title),
		Company:     e.Company,
		Location:    j.NormalizedLocation,
		Description: sanitizeHTML(j.Description),
		PostedAt:    parseLayout(amazonDateLayout, j.PostedDate),
	}
}
