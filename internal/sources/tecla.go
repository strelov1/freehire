package sources

import (
	"context"
	"fmt"
	"strconv"
)

// tecla adapts app.tecla.io, a remote-only marketplace of LatAm engineering vacancies. Its
// public getPublicJobs API returns a paginated, structured list — including each posting's
// own employer — so one paged list call assembles every Job with no per-job detail fetch.
// Unlike a single-company adapter, the company comes from the posting (the marketplace lists
// many employers), so its boardless config entry's company is only a validation placeholder.
// The public payload truncates the description (the full text is auth-gated and not fetched);
// the platform is remote-only, so every job is marked remote.
type tecla struct {
	http JSONGetter
}

const (
	teclaListURL = "https://api.tecla.io/api/jobs/getPublicJobs/?page=%d"
	teclaJobURL  = "https://app.tecla.io/job?id=%d"
	// teclaMaxPages bounds pagination so a wrong or missing countPages cannot loop.
	teclaMaxPages = 100
	// teclaDateLayout matches the zone-less createdAt tecla emits ("2026-05-25T17:02:00.864421").
	// The .9 fractional form parses any sub-second width (or none), so the digit count is not
	// load-bearing — the conventional 9-nine form just states "arbitrary precision".
	teclaDateLayout = "2006-01-02T15:04:05.999999999"
)

// NewTecla builds the Tecla adapter over the given HTTP client.
func NewTecla(c JSONGetter) Source { return tecla{http: c} }

func (tecla) Provider() string { return "tecla" }

// tecla is a marketplace with one global feed, so its config entries carry no board.
func (tecla) boardless() {}

// tecla aggregates postings from many companies, so it stays in the source facet.
func (tecla) aggregator() {}

// teclaResponse is the getPublicJobs response: Data.Jobs is the page, Data.Pagination.CountPages
// the total page count used to stop pagination.
type teclaResponse struct {
	Data struct {
		Jobs       []teclaJob `json:"jobs"`
		Pagination struct {
			CountPages int `json:"countPages"`
		} `json:"pagination"`
	} `json:"data"`
}

// teclaJob is one posting. Company nests the employer's own name (the marketplace lists many
// employers); Description is the truncated public text.
type teclaJob struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	CreatedAt   string `json:"createdAt"`
	Description string `json:"description"`
}

func (t tecla) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= teclaMaxPages; page++ {
		var resp teclaResponse
		if err := t.http.GetJSON(ctx, fmt.Sprintf(teclaListURL, page), &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("tecla: list page %d: %w", page, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		if len(resp.Data.Jobs) == 0 {
			break
		}
		for _, p := range resp.Data.Jobs {
			jobs = append(jobs, t.toJob(p))
		}
		if page >= resp.Data.Pagination.CountPages {
			break
		}
	}
	return jobs, nil
}

// toJob maps a posting to a Job. The company is the posting's own employer, not the
// configured entry; every job is remote (the marketplace is remote-only).
func (tecla) toJob(p teclaJob) Job {
	return Job{
		ExternalID:  strconv.Itoa(p.ID),
		URL:         fmt.Sprintf(teclaJobURL, p.ID),
		Title:       p.Name,
		Company:     p.Company.Name,
		Description: sanitizeHTML(p.Description),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parseLayout(teclaDateLayout, p.CreatedAt),
	}
}
