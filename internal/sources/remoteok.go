package sources

import (
	"context"
	"fmt"
)

// remoteokSource adapts remoteok.com, a remote-jobs aggregator. Boardless (one public
// API, no per-tenant board) and multi-company, so it stays in the source facet and takes
// each posting's company from the feed. The /api feed carries every posting's body inline
// (no detail call) but is capped at roughly the latest ~100 postings, so coverage is the
// recent window, not the full backlog.
//
// (The provider key is "remoteok"; the type is remoteokSource to avoid colliding with that
// string literal style elsewhere.)
type remoteokSource struct {
	http JSONGetter
}

const remoteokListURL = "https://remoteok.com/api"

// NewRemoteOK builds the RemoteOK adapter over the given HTTP client.
func NewRemoteOK(c JSONGetter) Source { return remoteokSource{http: c} }

func (remoteokSource) Provider() string { return "remoteok" }

func (remoteokSource) boardless() {}

func (remoteokSource) aggregator() {}

// remoteokPosting is one posting from the /api feed. The feed's first element is a legal
// notice (no id/slug); it is skipped by the empty-id guard in toJob.
type remoteokPosting struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Company     string `json:"company"`
	Position    string `json:"position"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Date        string `json:"date"`
	URL         string `json:"url"`
}

func (s remoteokSource) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var feed []remoteokPosting
	if err := s.http.GetJSON(ctx, remoteokListURL, &feed); err != nil {
		return nil, fmt.Errorf("remoteok: list: %w", err)
	}
	jobs := make([]Job, 0, len(feed))
	for _, p := range feed {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for the leading legal-notice
// element (no id) and any posting missing the company (which would break the slug). RemoteOK
// lists only remote jobs, so the work mode is always remote.
func (p remoteokPosting) toJob() (Job, bool) {
	if p.ID == "" || p.Company == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  p.ID,
		URL:         p.URL,
		Title:       p.Position,
		Company:     p.Company,
		Location:    p.Location,
		Description: sanitizeHTML(p.Description),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseRFC3339(p.Date),
	}, true
}
