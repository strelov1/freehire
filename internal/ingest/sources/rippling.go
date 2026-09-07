package sources

import (
	"context"
	"fmt"
)

// ripplingBaseURL is the Rippling public ATS board API root. The `ats.rippling.com`
// host is the live one and a superset: it serves every board the legacy
// `api.rippling.com/platform/api/ats/v1` host did, plus boards created on Rippling's
// newer ATS (e.g. `whitehatgaming`) that the legacy host answers 404 for. The path
// shape and JSON are identical across both, so this is a host swap and nothing more.
const (
	ripplingBaseURL = "https://ats.rippling.com/api/v1/board"
)

// rippling adapts the Rippling public ATS board API. Its list endpoint carries no
// description, so it fetches each posting's detail (bounded-concurrency) to assemble the
// body, like the SmartRecruiters adapter.
type rippling struct {
	http JSONGetter
}

// NewRippling builds the Rippling adapter over the given HTTP client.
func NewRippling(c JSONGetter) Source { return rippling{http: c} }

func (rippling) Provider() string { return "rippling" }

// ripplingPosting is one item from the board list (no description here).
type ripplingPosting struct {
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	WorkLocation struct {
		Label string `json:"label"`
	} `json:"workLocation"`
}

func (r rippling) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var postings []ripplingPosting
	if err := r.http.GetJSON(ctx, fmt.Sprintf("%s/%s/jobs", ripplingBaseURL, e.Board), &postings); err != nil {
		return nil, fmt.Errorf("rippling: list board %s: %w", e.Board, err)
	}

	// Each posting's description comes from its own detail request, fanned out under a
	// bounded worker pool.
	return fetchDetails(postings, defaultDetailWorkers, func(p ripplingPosting) (Job, bool) {
		return r.detail(ctx, e, p)
	}), nil
}

// detail fetches one posting's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that posting.
func (r rippling) detail(ctx context.Context, e CompanyEntry, p ripplingPosting) (Job, bool) {
	url := fmt.Sprintf("%s/%s/jobs/%s", ripplingBaseURL, e.Board, p.UUID)

	var d struct {
		CreatedOn   string `json:"createdOn"`
		Description struct {
			Role string `json:"role"` // company is boilerplate, not the role — excluded
		} `json:"description"`
	}
	if err := r.http.GetJSON(ctx, url, &d); err != nil {
		return Job{}, false
	}

	return Job{
		ExternalID:  p.UUID,
		URL:         p.URL,
		Title:       p.Name,
		Company:     e.Company,
		Location:    p.WorkLocation.Label,
		Description: sanitizeHTML(d.Description.Role),
		Remote:      isRemote(p.WorkLocation.Label),
		PostedAt:    parseRFC3339(d.CreatedOn),
	}, true
}
