package sources

import (
	"context"
	"fmt"
)

// jobicy adapts jobicy.com, a remote-jobs aggregator. Boardless (one public API, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed. The /api/v2/remote-jobs feed carries every posting's
// body inline (no detail call); count caps the response, so coverage is the recent window.
type jobicy struct {
	http JSONGetter
}

const (
	// jobicyCount is the feed's max page size (the API caps a single response at 50).
	jobicyCount   = 50
	jobicyListURL = "https://jobicy.com/api/v2/remote-jobs?count=%d"
)

// NewJobicy builds the Jobicy adapter over the given HTTP client.
func NewJobicy(c JSONGetter) Source { return jobicy{http: c} }

func (jobicy) Provider() string { return "jobicy" }

func (jobicy) boardless() {}

func (jobicy) aggregator() {}

// jobicyPosting is one posting from the /api/v2 feed, body inline (no detail call).
type jobicyPosting struct {
	ID             int64  `json:"id"`
	URL            string `json:"url"`
	JobTitle       string `json:"jobTitle"`
	CompanyName    string `json:"companyName"`
	JobGeo         string `json:"jobGeo"`
	JobDescription string `json:"jobDescription"`
	PubDate        string `json:"pubDate"`
}

func (s jobicy) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var resp struct {
		Jobs []jobicyPosting `json:"jobs"`
	}
	if err := s.http.GetJSON(ctx, fmt.Sprintf(jobicyListURL, jobicyCount), &resp); err != nil {
		return nil, fmt.Errorf("jobicy: list: %w", err)
	}
	jobs := make([]Job, 0, len(resp.Jobs))
	for _, p := range resp.Jobs {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// native id, or no company which would break the slug). Jobicy lists only remote jobs.
func (p jobicyPosting) toJob() (Job, bool) {
	if p.ID == 0 || p.CompanyName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  fmt.Sprintf("%d", p.ID),
		URL:         p.URL,
		Title:       p.JobTitle,
		Company:     p.CompanyName,
		Location:    p.JobGeo,
		Description: sanitizeHTML(p.JobDescription),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseRFC3339(p.PubDate),
	}, true
}
