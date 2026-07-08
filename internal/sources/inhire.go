package sources

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// inhire adapts InHire's public careers API (api.inhire.app), the ATS behind a number of
// Brazilian (Florianópolis-rooted) company career sites. A board is the tenant slug, passed
// on every request as the X-Tenant header. The listing call carries no description, so each
// post's body and publish date come from its own detail call, fanned out like the other
// detail-fetching adapters.
type inhire struct {
	http HeaderJSONGetter
}

const (
	inhireListURL    = "https://api.inhire.app/job-posts/public/pages"
	inhireDetailURL  = "https://api.inhire.app/job-posts/public/pages/%s"
	inhireVacancyURL = "https://%s.inhire.app/vagas/%s"
)

// NewInhire builds the InHire adapter over the given HTTP client.
func NewInhire(c HeaderJSONGetter) Source { return inhire{http: c} }

func (inhire) Provider() string { return "inhire" }

// inhirePost is one job post from the listing (no description here).
type inhirePost struct {
	JobID         string `json:"jobId"`
	DisplayName   string `json:"displayName"`
	WorkplaceType string `json:"workplaceType"`
	Location      string `json:"location"`
	Status        string `json:"status"`
}

func (h inhire) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	posts, err := h.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}

	return fetchDetails(posts, defaultDetailWorkers, func(p inhirePost) (Job, bool) {
		return h.detail(ctx, e, p)
	}), nil
}

// tenantHeader is the per-tenant routing header every InHire call carries.
func tenantHeader(board string) map[string]string {
	return map[string]string{"X-Tenant": board}
}

// list fetches a board's job-post list (no pagination).
func (h inhire) list(ctx context.Context, board string) ([]inhirePost, error) {
	var resp struct {
		JobsPage []inhirePost `json:"jobsPage"`
	}
	if err := h.http.GetJSONWithHeaders(ctx, inhireListURL, tenantHeader(board), &resp); err != nil {
		return nil, fmt.Errorf("inhire: list board %s: %w", board, err)
	}
	return resp.JobsPage, nil
}

// detail fetches one post's body and publish date and maps it to a Job, returning ok=false
// when the fetch or decode fails so the caller skips just that post.
func (h inhire) detail(ctx context.Context, e CompanyEntry, p inhirePost) (Job, bool) {
	var d struct {
		Description string `json:"description"`
		PublishedAt string `json:"publishedAt"`
		CreatedAt   string `json:"createdAt"`
	}
	url := fmt.Sprintf(inhireDetailURL, p.JobID)
	if err := h.http.GetJSONWithHeaders(ctx, url, tenantHeader(e.Board), &d); err != nil {
		return Job{}, false
	}

	mode := workplaceTypeMode(p.WorkplaceType)
	return Job{
		ExternalID:  p.JobID,
		URL:         fmt.Sprintf(inhireVacancyURL, e.Board, p.JobID),
		Title:       strings.TrimSpace(p.DisplayName),
		Company:     e.Company,
		Location:    p.Location,
		Description: sanitizeHTML(html.UnescapeString(d.Description)),
		Remote:      mode == "remote",
		PostedAt:    parseRFC3339(firstNonEmpty(d.PublishedAt, d.CreatedAt)),
		WorkMode:    mode,
	}, true
}
