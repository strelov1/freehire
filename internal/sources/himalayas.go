package sources

import (
	"context"
	"fmt"
)

// himalayas adapts himalayas.app, a remote-jobs aggregator. Boardless (one public API, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed. The /jobs/api endpoint pages by offset/limit over a
// reported totalCount; the site is remote-only, so every job is remote.
type himalayas struct {
	http JSONGetter
}

const (
	// himalayasLimit is the page size requested per offset page.
	himalayasLimit = 100
	// himalayasMaxPages caps pagination so a runaway/over-reported totalCount cannot loop
	// indefinitely (the same defensive bound the other paginating adapters carry).
	himalayasMaxPages = 400
	himalayasListURL  = "https://himalayas.app/jobs/api?limit=%d&offset=%d"
)

// NewHimalayas builds the Himalayas adapter over the given HTTP client.
func NewHimalayas(c JSONGetter) Source { return himalayas{http: c} }

func (himalayas) Provider() string { return "himalayas" }

func (himalayas) boardless() {}

func (himalayas) aggregator() {}

// himalayasResponse is one offset page: the postings plus the catalogue-wide total used to
// decide whether another page is due.
type himalayasResponse struct {
	TotalCount int                `json:"totalCount"`
	Jobs       []himalayasPosting `json:"jobs"`
}

// himalayasPosting is one posting, body inline (no detail call). pubDate is epoch seconds.
type himalayasPosting struct {
	Title                string   `json:"title"`
	CompanyName          string   `json:"companyName"`
	ApplicationLink      string   `json:"applicationLink"`
	GUID                 string   `json:"guid"`
	LocationRestrictions []string `json:"locationRestrictions"`
	Description          string   `json:"description"`
	PubDate              int64    `json:"pubDate"`
}

func (s himalayas) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for offset, page := 0, 0; page < himalayasMaxPages; offset, page = offset+himalayasLimit, page+1 {
		var resp himalayasResponse
		url := fmt.Sprintf(himalayasListURL, himalayasLimit, offset)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("himalayas: list offset %d: %w", offset, err)
		}
		for _, p := range resp.Jobs {
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		// Stop once this page's offset covers the reported total, or the page came back
		// empty (defensive against a total that never shrinks).
		if len(resp.Jobs) == 0 || offset+himalayasLimit >= resp.TotalCount {
			break
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// guid to key on, or no company which would break the slug). Himalayas lists only remote jobs.
func (p himalayasPosting) toJob() (Job, bool) {
	if p.GUID == "" || p.CompanyName == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  p.GUID,
		URL:         p.ApplicationLink,
		Title:       p.Title,
		Company:     p.CompanyName,
		Location:    joinNonEmpty(p.LocationRestrictions...),
		Description: sanitizeHTML(p.Description),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseEpochSeconds(p.PubDate),
	}, true
}
