package sources

import (
	"context"
	"fmt"
	"strings"
)

// lumenalta adapts Lumenalta's public careers API (lumenalta.com/api/jobs), a single-company
// source with no per-tenant board id (boardless). The list endpoint returns each posting —
// including the full plain-text description — inline, so one paged call assembles every Job
// with no per-job detail fetch. Lumenalta is a remote-only consultancy, so its postings carry
// no location field; the catalogue is paged and meta.total bounds it.
type lumenalta struct {
	http JSONGetter
}

const (
	lumenaltaListURL   = "https://lumenalta.com/api/jobs?page=%d&limit=%d"
	lumenaltaJobURL    = "https://lumenalta.com/careers/"
	lumenaltaPageLimit = 100
)

// NewLumenalta builds the Lumenalta adapter over the given HTTP client.
func NewLumenalta(c JSONGetter) Source { return lumenalta{http: c} }

func (lumenalta) Provider() string { return "lumenalta" }

// lumenalta is single-company, so its config entries carry no board.
func (lumenalta) boardless() {}

// lumenaltaResponse is the list response. Meta.Total is the catalogue size, used to stop
// paging; Data is empty past the last page.
type lumenaltaResponse struct {
	Data []lumenaltaJob `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

// lumenaltaJob is one posting from the list response. Description is plain text and the
// public page URL is built from slug.
type lumenaltaJob struct {
	ID          string `json:"_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (l lumenalta) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		var resp lumenaltaResponse
		url := fmt.Sprintf(lumenaltaListURL, page, lumenaltaPageLimit)
		if err := l.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("lumenalta: list page %d: %w", page, err)
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, j := range resp.Data {
			jobs = append(jobs, l.toJob(e, j))
		}
		if len(jobs) >= resp.Meta.Total {
			break
		}
	}
	return jobs, nil
}

// toJob maps a list result to a Job. Lumenalta is remote-only and exposes no location, so the
// location is set to "Remote" (which the pipeline's dictionary derives a remote work-mode from).
func (lumenalta) toJob(e CompanyEntry, j lumenaltaJob) Job {
	return Job{
		ExternalID: j.ID,
		URL:        lumenaltaJobURL + j.Slug,
		Title:      strings.TrimSpace(j.Name),
		Company:    e.Company,
		Location:   "Remote",
		// The API serves the description as plain text; rebuild it into sanitized structural
		// HTML so the {@html} consumer renders paragraphs/lists instead of one collapsed line.
		Description: sanitizeHTML(plainTextToHTML(j.Description)),
		Remote:      true,
	}
}
