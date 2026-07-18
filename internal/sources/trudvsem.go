package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// trudvsem adapts opendata.trudvsem.ru — the open-data API of "Работа России", the Russian
// federal employment service's job board. It is keyless. Postings are enumerated per federal
// subject (region), carried as the board file entry's board: a 13-digit OKATO region code
// (e.g. 7700000000000 = Moscow). The API hard-caps pagination — the offset param is a 0-based
// page index, page size maxes at 100, and page 100 errors — so a region with more than ~10k
// open vacancies yields only its freshest 10k (results are date_modify-descending; the lost
// tail is the stalest postings), while every smaller region is exhausted in full. Multi-company
// (employer per posting), board-based like arbeitsagentur — no new source shape needed.
type trudvsem struct {
	http JSONGetter
}

const (
	trudvsemRegionURL = "https://opendata.trudvsem.ru/api/v1/vacancies/region/%s"
	trudvsemPageSize  = 100
	// trudvsemMaxPages caps the page loop at the API's pagination ceiling. The offset param is
	// a 0-based PAGE index (not a record offset); page index 100 (offset=100) errors with a 500,
	// so at most 100 pages — the freshest ~10k postings — are reachable per region.
	trudvsemMaxPages = 100
)

// NewTrudvsem builds the Trudvsem adapter over the given JSON client.
func NewTrudvsem(c JSONGetter) Source { return trudvsem{http: c} }

func (trudvsem) Provider() string { return "trudvsem" }

// trudvsemResponse is one vacancies page. A page past the end of a region's results 500s
// (surfaced as a transport error), so the loop stops before requesting one by watching the
// per-page count and the reported total.
type trudvsemResponse struct {
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Results struct {
		Vacancies []struct {
			Vacancy trudvsemVacancy `json:"vacancy"`
		} `json:"vacancies"`
	} `json:"results"`
}

// trudvsemVacancy is one posting. Only the fields the adapter maps are decoded.
type trudvsemVacancy struct {
	ID     string `json:"id"`
	Region struct {
		Name string `json:"name"`
	} `json:"region"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	CreationDate string `json:"creation-date"`
	JobName      string `json:"job-name"`
	VacURL       string `json:"vac_url"`
	Employment   string `json:"employment"`
	Requirements string `json:"requirements"`
	Duty         string `json:"duty"`
}

// trudvsemEmployment maps Trudvsem's employment enum to freehire's controlled EmploymentType
// vocabulary. Only the confidently-known values are listed; an unrecognized value maps to ""
// so the dictionaries decide, rather than guessing at a mapping the board does not warrant
// (e.g. temporary/seasonal has no clean equivalent).
var trudvsemEmployment = map[string]string{
	"Полная занятость":    "full_time",
	"Частичная занятость": "part_time",
	"Стажировка":          "internship",
}

func (t trudvsem) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 0; page < trudvsemMaxPages; page++ {
		var resp trudvsemResponse
		if err := t.http.GetJSON(ctx, t.pageURL(e.Board, page), &resp); err != nil {
			return nil, fmt.Errorf("trudvsem: region %q page %d: %w", e.Board, page, err)
		}
		for _, w := range resp.Results.Vacancies {
			if j, ok := t.toJob(w.Vacancy, e.Company); ok {
				jobs = append(jobs, j)
			}
		}
		// Stop at a short/empty page (the last one) or once the reported total is covered — the
		// API pages by index, so after this page we have seen (page+1)*pageSize records. Both
		// guards keep the loop from requesting a page past the end, which 500s.
		if len(resp.Results.Vacancies) < trudvsemPageSize || (page+1)*trudvsemPageSize >= resp.Meta.Total {
			break
		}
	}
	return jobs, nil
}

// pageURL builds a region-shard request for the given 0-based page index (the API's offset
// param is a page number, not a record offset).
func (trudvsem) pageURL(region string, page int) string {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(trudvsemPageSize))
	q.Set("offset", strconv.Itoa(page))
	return fmt.Sprintf(trudvsemRegionURL, region) + "?" + q.Encode()
}

// toJob maps a vacancy to a Job, dropping one with no id or blank title. hubName is the board
// entry's company, used only as the employer fallback when a posting omits its own.
func (trudvsem) toJob(v trudvsemVacancy, hubName string) (Job, bool) {
	if v.ID == "" || strings.TrimSpace(v.JobName) == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:     v.ID,
		URL:            strings.TrimSpace(v.VacURL),
		Title:          strings.TrimSpace(v.JobName),
		Company:        firstNonEmpty(strings.TrimSpace(v.Company.Name), hubName),
		Location:       strings.TrimSpace(v.Region.Name),
		Description:    trudvsemDescription(v),
		EmploymentType: trudvsemEmployment[strings.TrimSpace(v.Employment)],
		PostedAt:       parseDate(strings.TrimSpace(v.CreationDate)),
	}, true
}

// trudvsemDescription assembles the posting body from its duty and requirements text — both
// plain text with no markup — under Russian headings, then rebuilds paragraph/list structure
// via plainTextToHTML so it does not render as one unbroken block. An empty result is fine:
// a description-less posting is still worth emitting.
func trudvsemDescription(v trudvsemVacancy) string {
	var b strings.Builder
	if duty := strings.TrimSpace(v.Duty); duty != "" {
		b.WriteString("Обязанности:\n" + duty)
	}
	if req := strings.TrimSpace(v.Requirements); req != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Требования:\n" + req)
	}
	if b.Len() == 0 {
		return ""
	}
	return sanitizeHTML(plainTextToHTML(b.String()))
}
