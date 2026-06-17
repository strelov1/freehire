package sources

import (
	"context"
	"fmt"
)

// remotive adapts remotive.com, a remote-jobs aggregator. Boardless (one public API, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed. The /api/remote-jobs feed carries every posting's body
// inline and is fetched ONCE per run: the API is rate-limited (data is 24h-delayed and the
// provider asks for at most a few requests a day), so the adapter never paginates or polls.
// Remotive lists only remote jobs.
type remotive struct {
	http JSONGetter
}

const remotiveListURL = "https://remotive.com/api/remote-jobs"

// remotivePubLayout is Remotive's publication_date format: ISO 8601 with no timezone
// ("2026-06-16T06:59:30"), so RFC3339 parsing would reject it.
const remotivePubLayout = "2006-01-02T15:04:05"

// NewRemotive builds the Remotive adapter over the given HTTP client.
func NewRemotive(c JSONGetter) Source { return remotive{http: c} }

func (remotive) Provider() string { return "remotive" }

func (remotive) boardless() {}

func (remotive) aggregator() {}

// remotivePosting is one posting from the /api/remote-jobs feed, body inline (no detail call).
type remotivePosting struct {
	ID                        int64  `json:"id"`
	URL                       string `json:"url"`
	Title                     string `json:"title"`
	CompanyName               string `json:"company_name"`
	CandidateRequiredLocation string `json:"candidate_required_location"`
	Description               string `json:"description"`
	PublicationDate           string `json:"publication_date"`
}

func (s remotive) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var resp struct {
		Jobs []remotivePosting `json:"jobs"`
	}
	if err := s.http.GetJSON(ctx, remotiveListURL, &resp); err != nil {
		return nil, fmt.Errorf("remotive: list: %w", err)
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
// native id, or no company which would break the slug). Remotive lists only remote jobs.
func (p remotivePosting) toJob() (Job, bool) {
	if p.ID == 0 || p.CompanyName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  fmt.Sprintf("%d", p.ID),
		URL:         p.URL,
		Title:       p.Title,
		Company:     p.CompanyName,
		Location:    p.CandidateRequiredLocation,
		Description: sanitizeHTML(p.Description),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseLayout(remotivePubLayout, p.PublicationDate),
	}, true
}
