package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// rostelecom adapts Rostelecom's careers API (job.rt.ru), a single-company source with no
// per-tenant board id (boardless). Like sber, the listing carries each vacancy's body inline
// (whatWeToDo / whatWeExpect / whatWeOffer), so a crawl is the listing alone. The API names
// no employer entity, so every posting is the configured entry's company, and it states no
// publication date.
type rostelecom struct {
	http JSONGetter
}

const (
	// rostelecomListURL pages from 0. size is honoured (pageSize/limit are not).
	rostelecomListURL = "https://job.rt.ru/backend/api/vacancies?page=%d&size=%d"
	// rostelecomJobURL is the public posting page — the site's Next.js route
	// /vacancy/[vacancyId]; the plural /vacancies/{id} 404s.
	rostelecomJobURL  = "https://job.rt.ru/vacancy/%d"
	rostelecomPerPage = 100
	// rostelecomMaxPages is a safety ceiling: reaching it before totalPages fails the crawl.
	rostelecomMaxPages = 50
)

// NewRostelecom builds the Rostelecom adapter over the given HTTP client.
func NewRostelecom(c JSONGetter) Source { return rostelecom{http: c} }

func (rostelecom) Provider() string { return "rostelecom" }

// rostelecom is single-company, so its config entry carries no board.
func (rostelecom) boardless() {}

// rostelecomNamed is the {id, name} pair the API uses for cities, directions and tags.
type rostelecomNamed struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// rostelecomVacancy is one listed vacancy with its body inline. The payload's salaryCurrency
// is not read: it is "руб" or null (even when a figure is stated), so it adds nothing to the
// rouble default. The street address is not read either: the city is the location, and a
// street line only gives the location parser something to misread.
type rostelecomVacancy struct {
	ID           int               `json:"id"`
	Name         string            `json:"name"`
	SalaryFrom   *float64          `json:"salaryFrom"`
	SalaryTo     *float64          `json:"salaryTo"`
	Directions   []rostelecomNamed `json:"directions"`
	City         *rostelecomNamed  `json:"city"`
	Tags         []rostelecomNamed `json:"tags"`
	WhatWeToDo   string            `json:"whatWeToDo"`
	WhatWeExpect string            `json:"whatWeExpect"`
	WhatWeOffer  string            `json:"whatWeOffer"`
}

func (r rostelecom) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	vacancies, err := r.list(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(vacancies))
	for _, v := range vacancies {
		if v.ID == 0 || strings.TrimSpace(v.Name) == "" {
			continue
		}
		jobs = append(jobs, v.toJob(e))
	}
	return jobs, nil
}

// list pages through every vacancy until totalPages, deduplicating by id across pages. The
// crawl must PROVE it read the whole listing, because a short one reads as closed postings to
// the unseen sweep: any page failing, the page cap reached before the last page, or fewer
// distinct ids than the reply's own totalCount (a page that came back empty or overlapping)
// all fail the crawl rather than return what was gathered.
func (r rostelecom) list(ctx context.Context) ([]rostelecomVacancy, error) {
	var all []rostelecomVacancy
	seen := map[int]bool{}
	totalCount := 0
	for page := 0; ; page++ {
		if page >= rostelecomMaxPages {
			return nil, fmt.Errorf("rostelecom: page cap %d reached before the end of the listing", rostelecomMaxPages)
		}
		var resp struct {
			TotalPages int                 `json:"totalPages"`
			TotalCount int                 `json:"totalCount"`
			Vacancies  []rostelecomVacancy `json:"vacancies"`
		}
		if err := r.http.GetJSON(ctx, fmt.Sprintf(rostelecomListURL, page, rostelecomPerPage), &resp); err != nil {
			return nil, fmt.Errorf("rostelecom: list page %d: %w", page, err)
		}
		totalCount = max(totalCount, resp.TotalCount)
		for _, v := range resp.Vacancies {
			if !seen[v.ID] {
				seen[v.ID] = true
				all = append(all, v)
			}
		}
		if len(resp.Vacancies) == 0 || page+1 >= resp.TotalPages {
			break
		}
	}
	if len(all) < totalCount {
		return nil, fmt.Errorf("rostelecom: listing yielded %d distinct postings, totalCount says %d", len(all), totalCount)
	}
	return all, nil
}

// Rostelecom's tag dictionary (GET /backend/api/dictionaries/tags) ids the adapter reads.
const (
	rostelecomTagPartDay   = 3  // Неполный день
	rostelecomTagPartTime  = 10 // Частичная занятость
	rostelecomTagHybrid    = 11 // Гибрид
	rostelecomDirNoExpName = "Без опыта"
)

// rostelecomTechDirections are the directions that state a posting is technical. "Разработка"
// does not name WHICH discipline, so it asserts IT-ness only; "ИБ" pins one category. The rest
// ("Технический блок" is field and telecom engineering, "Аналитика" spans business analysis)
// assert nothing and leave the title dictionary to decide.
var rostelecomTechDirections = map[string]string{
	"Разработка": "",
	"ИБ":         "security",
}

// toJob maps a listed vacancy to a Job.
func (v rostelecomVacancy) toJob(e CompanyEntry) Job {
	job := Job{
		ExternalID: strconv.Itoa(v.ID),
		URL:        fmt.Sprintf(rostelecomJobURL, v.ID),
		Title:      v.Name,
		Company:    e.Company,
		Description: sanitizeHTML(
			headedSection("Чем предстоит заниматься", v.WhatWeToDo) +
				headedSection("Что мы ожидаем", v.WhatWeExpect) +
				headedSection("Что мы предлагаем", v.WhatWeOffer)),
	}
	if v.City != nil {
		job.Location = strings.TrimSpace(v.City.Name)
	}
	job.Remote = isRemote(job.Location)

	for _, t := range v.Tags {
		switch t.ID {
		case rostelecomTagHybrid:
			job.WorkMode = "hybrid"
		case rostelecomTagPartDay, rostelecomTagPartTime:
			job.EmploymentType = "part_time"
		}
	}
	for _, d := range v.Directions {
		if cat, ok := rostelecomTechDirections[d.Name]; ok {
			job.IsTechHint = true
			if cat != "" {
				job.Category = cat
			}
		}
		if d.Name == rostelecomDirNoExpName {
			zero := 0
			job.ExperienceYearsMin = &zero
		}
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = rubMonthlySalary(v.SalaryFrom, v.SalaryTo)
	return job
}
