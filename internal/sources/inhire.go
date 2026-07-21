package sources

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
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
	inhireListURL   = "https://api.inhire.app/job-posts/public/pages"
	inhireDetailURL = "https://api.inhire.app/job-posts/public/pages/%s"
	// inhireVacancyURL is the public vacancy page. InHire's careers SPA routes on
	// /vagas/:jobId/:slug and renders a blank page when the slug segment is absent, so the
	// URL must carry a trailing slug. The segment is cosmetic — the SPA fetches the posting
	// by jobId alone — so any non-empty slug renders the vacancy.
	inhireVacancyURL = "https://%s.inhire.app/vagas/%s/%s"
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

// inhireVacancyURLFor builds the public vacancy URL, deriving the required trailing slug
// from the title. The slug is cosmetic (see inhireVacancyURL), so a transliterated title
// slug is enough; "vaga" stands in when the title yields no slug so the segment is never
// empty — an empty segment collapses the URL back to /vagas/:jobId, which renders blank.
func inhireVacancyURLFor(board, jobID, title string) string {
	slug := normalize.Slug(title)
	if slug == "" {
		slug = "vaga"
	}
	return fmt.Sprintf(inhireVacancyURL, board, jobID, slug)
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
	title := strings.TrimSpace(p.DisplayName)
	return Job{
		ExternalID:  p.JobID,
		URL:         inhireVacancyURLFor(e.Board, p.JobID, title),
		Title:       title,
		Company:     e.Company,
		Location:    p.Location,
		Description: sanitizeHTML(html.UnescapeString(d.Description)),
		Remote:      mode == "remote",
		PostedAt:    parseRFC3339(firstNonEmpty(d.PublishedAt, d.CreatedAt)),
		WorkMode:    mode,
	}, true
}
