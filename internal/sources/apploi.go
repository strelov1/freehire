package sources

import (
	"context"
	"fmt"
)

// apploi adapts the apploi.com job board (jobs.apploi.com), a healthcare-focused ATS. The
// board is the numeric employer id. The public API lists an employer's postings, descriptions
// inline, at api.apploi.com/v1/jobs?employer=<id> (limit/offset paginated) — so a whole board
// comes from paginated list calls with no per-posting detail fetch.
type apploi struct {
	http JSONGetter
}

const (
	apploiAPI    = "https://api.apploi.com/v1/jobs"
	apploiJobURL = "https://jobs.apploi.com/view/%s"
	// apploiPageSize is the page window; apploiMaxPages caps the walk so a board that never
	// short-returns cannot loop forever (boards run to a few hundred postings at most).
	apploiPageSize = 100
	apploiMaxPages = 100
)

// NewApploi builds the apploi.com adapter over the given JSON client.
func NewApploi(c JSONGetter) Source { return apploi{http: c} }

func (apploi) Provider() string { return "apploi" }

func (s apploi) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 0; page < apploiMaxPages; page++ {
		url := fmt.Sprintf("%s?employer=%s&limit=%d&offset=%d", apploiAPI, e.Board, apploiPageSize, page*apploiPageSize)
		var resp struct {
			Data []apploiJob `json:"data"`
		}
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("apploi: board %q: %w", e.Board, err)
			}
			break // a later page failing just stops the walk with what we have
		}
		for _, j := range resp.Data {
			// The list carries archived/unpublished/private rows too; keep only the live,
			// publicly searchable postings.
			if !j.Published || j.Archived || j.Private || j.ID == "" {
				continue
			}
			jobs = append(jobs, s.toJob(e, j))
		}
		if len(resp.Data) < apploiPageSize {
			break // last (short) page
		}
	}
	return jobs, nil
}

func (apploi) toJob(e CompanyEntry, j apploiJob) Job {
	location := joinNonEmpty(j.City, j.State, j.Country)
	return Job{
		ExternalID:  j.ID,
		URL:         fmt.Sprintf(apploiJobURL, j.ID),
		Title:       j.Name,
		Company:     firstNonEmpty(e.Company, j.BrandName),
		Location:    location,
		Description: sanitizeHTML(j.Description),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(j.PublishedDate),
	}
}

// apploiJob is one posting in the api.apploi.com/v1/jobs list. Descriptions are inline, so
// no detail fetch is needed.
type apploiJob struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	PublishedDate string `json:"published_date"`
	BrandName     string `json:"brand_name_with_company_only"`
	Published     bool   `json:"published"`
	Archived      bool   `json:"archived"`
	Private       bool   `json:"private"`
}
