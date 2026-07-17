package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// solidesBaseURL is the Solides public jobs API. Solides is a broad Brazilian ATS;
// a company's career page lives at <slug>.vagas.solides.com.br, but the listing API
// is this one central gateway host keyed by the tenant slug.
const solidesBaseURL = "https://apigw.solides.com.br/jobs/v3/home/vacancy"

// solidesPageSize is the listing page size. The server caps take at 25, so this is
// also the maximum.
const solidesPageSize = 25

// solidesMaxPages bounds the page walk. The stop signal is the response's totalPages,
// but this caps a misbehaving API at 25000 postings — well above any single tenant.
const solidesMaxPages = 1000

// solidesOpenState is the currentState value marking an open vacancy; any other
// present value (e.g. "encerrada") means the posting is closed and is skipped.
const solidesOpenState = "em_andamento"

// solides adapts the Solides jobs API. Its list endpoint carries the description
// inline, so — like Gupy — it needs no per-posting detail request. The board id is
// the tenant's subdomain slug.
type solides struct {
	http JSONGetter
}

// NewSolides builds the Solides adapter over the given HTTP client.
func NewSolides(c JSONGetter) Source { return solides{http: c} }

func (solides) Provider() string { return "solides" }

// solidesJob is one posting from the Solides listing. The description is full HTML
// inline; city/state are nested objects; homeOffice is a structured remote flag and
// jobType a structured "remoto"/"hibrido"/"presencial" enum.
type solidesJob struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CurrentState string `json:"currentState"`
	City         struct {
		Name string `json:"name"`
	} `json:"city"`
	State struct {
		Code string `json:"code"`
	} `json:"state"`
	HomeOffice bool   `json:"homeOffice"`
	JobType    string `json:"jobType"`
	CreatedAt  string `json:"createdAt"`
}

func (s solides) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		// Skip a posting whose state is present and not open; a missing state is
		// treated as open (the API only emits the field on real vacancies).
		if p.CurrentState != "" && p.CurrentState != solidesOpenState {
			continue
		}
		jobs = append(jobs, Job{
			ExternalID:  strconv.FormatInt(p.ID, 10),
			URL:         fmt.Sprintf("https://%s.vagas.solides.com.br/vaga/%d", e.Board, p.ID),
			Title:       strings.TrimSpace(p.Title),
			Company:     e.Company,
			Location:    joinNonEmpty(p.City.Name, p.State.Code),
			Description: sanitizeHTML(p.Description),
			Remote:      p.HomeOffice,
			WorkMode:    solidesWorkMode(p.JobType),
			PostedAt:    parseDate(p.CreatedAt),
		})
	}
	return jobs, nil
}

// list pages through the tenant's postings, stopping at the response's totalPages.
// A first-page failure aborts (the board yields nothing); a later-page failure after
// at least one page succeeded stops the walk and returns what was gathered, so a
// transient mid-walk error does not discard the postings already collected.
func (s solides) list(ctx context.Context, board string) ([]solidesJob, error) {
	var postings []solidesJob
	for page := 1; page <= solidesMaxPages; page++ {
		url := fmt.Sprintf("%s?slug=%s&take=%d&page=%d", solidesBaseURL, board, solidesPageSize, page)
		var resp struct {
			Data struct {
				TotalPages int          `json:"totalPages"`
				Data       []solidesJob `json:"data"`
			} `json:"data"`
		}
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("solides: list tenant %s: %w", board, err)
			}
			break // partial: keep what earlier pages yielded
		}
		postings = append(postings, resp.Data.Data...)
		if page >= resp.Data.TotalPages {
			break // reached the last page the server reports
		}
	}
	return postings, nil
}

// solidesWorkMode maps Solides's jobType enum to our work-mode vocabulary; an
// unrecognized value yields "".
func solidesWorkMode(jobType string) string {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "remoto":
		return "remote"
	case "hibrido":
		return "hybrid"
	case "presencial":
		return "onsite"
	default:
		return ""
	}
}
