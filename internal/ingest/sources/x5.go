package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// x5 adapts X5 Group's career site (rabota.x5.ru), a single-company source with no per-tenant
// board id (boardless). Its public vacancy API pages a list that carries every structured fact
// (city, work format, employment type, category, business unit, salary, publish date) but no
// body; the body lives on a per-vacancy detail request. So x5 is a HydratingSource: a posting
// the catalogue already has is re-listed as a liveness refresh and costs no detail request.
//
// X5 hires for many retail chains and divisions (Пятёрочка, Перекрёсток, Чижик, X5 Tech, ...),
// and each vacancy names its own as a business unit, so that is the posting's employer, with
// the configured entry as the fallback — the same choice sber and mts make for their entities.
//
// A second API (/public/api/vacancies/vacancies/) serves store-floor hourly vacancies; it is
// the shop-staff funnel and is deliberately not read.
type x5 struct {
	http JSONGetter
}

const (
	x5ListURL    = "https://rabota.x5.ru/vacancy/api/v1/public/vacancies?page=%d&page_size=%d"
	x5DetailURL  = "https://rabota.x5.ru/vacancy/api/v1/public/vacancies/%s"
	x5VacancyURL = "https://rabota.x5.ru/vacancies/%s"
	x5PageSize   = 100
	// x5ITCategory is the name of the platform's own top-level IT category.
	x5ITCategory = "IT"
)

// NewX5 builds the X5 Group adapter over the given HTTP client.
func NewX5(c JSONGetter) Source { return x5{http: c} }

func (x5) Provider() string { return "x5" }

// x5 is single-company, so its config entries carry no board.
func (x5) boardless() {}

// x5Item is one vacancy as the list returns it — everything but the body.
type x5Item struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	City           string   `json:"city"`
	Salary         *float64 `json:"salary"`
	SalaryMin      *float64 `json:"salary_min"`
	SalaryMax      *float64 `json:"salary_max"`
	EmploymentType string   `json:"employment_type"`
	WorkFormat     string   `json:"work_format"`
	WorkSchedule   string   `json:"work_schedule"`
	KeyFeatures    []string `json:"key_features"`
	Category       *struct {
		Parent *struct {
			Name string `json:"name"`
		} `json:"parent"`
	} `json:"category"`
	BusinessUnits []struct {
		BusinessUnit struct {
			Name string `json:"name"`
		} `json:"business_unit"`
	} `json:"business_units"`
	Address     string `json:"address"`
	PublishedAt string `json:"published_at"`
}

// x5Detail is the part of the detail response the list does not already carry. Every content
// field is optional; most vacancies fill only main_responsibilities and professional_skills.
type x5Detail struct {
	Content struct {
		Description          string `json:"description"`
		MainResponsibilities string `json:"main_responsibilities"`
		ProfessionalSkills   string `json:"professional_skills"`
		Education            string `json:"education"`
		Experience           string `json:"experience"`
		SoftwareSkills       string `json:"software_skills"`
		ForeignLanguage      string `json:"foreign_language"`
		PersonalQualities    string `json:"personal_qualities"`
		OtherRequirements    string `json:"other_requirements"`
		WorkingConditions    string `json:"working_conditions"`
		BusinessTrips        string `json:"business_trips"`
		CharacterWork        bool   `json:"character_work"`
	} `json:"content"`
	Office *struct {
		Address struct {
			FullAddress string `json:"full_address"`
			City        string `json:"city"`
		} `json:"address"`
	} `json:"office"`
}

// Fetch is the list-only fallback HydratingSource specifies for a caller that cannot supply a
// seen set; it hydrates every posting. FetchNew is the path cmd/ingest drives.
func (s x5) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every vacancy and fetches a detail only for an id the catalogue does not
// already have. A seen posting is emitted as a liveness refresh (identity only); re-upserting
// it content-less would wipe the description and the facets derived from it.
func (s x5) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(items, defaultDetailWorkers, func(it x5Item) (Job, bool) {
		if it.ID == "" {
			return Job{}, false // no native id → would collide on the dedup key; skip it
		}
		if seen(it.ID) {
			return Job{
				ExternalID:  it.ID,
				URL:         fmt.Sprintf(x5VacancyURL, it.ID),
				Title:       it.Name,
				Company:     x5Company(e, it),
				SeenRefresh: true,
			}, true
		}
		return s.detail(ctx, e, it)
	}), nil
}

// list walks every page to the response's own page count. Any page failing fails the crawl:
// the list is the only enumeration, and a silently short one reads as postings gone.
func (s x5) list(ctx context.Context) ([]x5Item, error) {
	var items []x5Item
	for page := 1; ; page++ {
		var resp struct {
			Pages int      `json:"pages"`
			Items []x5Item `json:"items"`
		}
		if err := s.http.GetJSON(ctx, fmt.Sprintf(x5ListURL, page, x5PageSize), &resp); err != nil {
			return nil, fmt.Errorf("x5: list page %d: %w", page, err)
		}
		items = append(items, resp.Items...)
		if page >= resp.Pages || len(resp.Items) == 0 {
			return items, nil
		}
	}
}

// detail fetches one vacancy's body and maps it, with the list item's structured facts, to a
// Job. ok=false when the detail request fails, so the caller skips just that vacancy; the
// next crawl offers it again, since it was never stored.
func (s x5) detail(ctx context.Context, e CompanyEntry, it x5Item) (Job, bool) {
	var d x5Detail
	if err := s.http.GetJSON(ctx, fmt.Sprintf(x5DetailURL, it.ID), &d); err != nil {
		return Job{}, false
	}

	location := it.City
	if location == "" && d.Office != nil {
		location = firstNonEmpty(d.Office.Address.City, d.Office.Address.FullAddress)
	}
	location = firstNonEmpty(location, it.Address)
	workMode := x5WorkMode(it.WorkFormat)
	salaryMin, salaryMax, currency, period := x5Salary(it)

	return Job{
		ExternalID:     it.ID,
		URL:            fmt.Sprintf(x5VacancyURL, it.ID),
		Title:          it.Name,
		Company:        x5Company(e, it),
		Location:       location,
		Description:    sanitizeHTML(x5Description(it, d)),
		Remote:         workMode == "remote",
		WorkMode:       workMode,
		EmploymentType: x5EmploymentType(it.EmploymentType),
		EducationLevel: x5EducationLevels[strings.TrimSpace(d.Content.Education)],
		IsTechHint:     it.Category != nil && it.Category.Parent != nil && it.Category.Parent.Name == x5ITCategory,
		SalaryMin:      salaryMin,
		SalaryMax:      salaryMax,
		SalaryCurrency: currency,
		SalaryPeriod:   period,
		PostedAt:       parseRFC3339(it.PublishedAt),
	}, true
}

// x5Company is the vacancy's own business unit, else the configured entry. Every live vacancy
// names exactly one.
func x5Company(e CompanyEntry, it x5Item) string {
	for _, bu := range it.BusinessUnits {
		if name := strings.TrimSpace(bu.BusinessUnit.Name); name != "" {
			return name
		}
	}
	return e.Company
}

// x5WorkMode maps the platform's work_format enum. "office" and "on_site" ("На месте
// работодателя" on the site) are both on-site work.
func x5WorkMode(format string) string {
	switch format {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "office", "on_site":
		return "onsite"
	default:
		return ""
	}
}

// x5EmploymentType maps the platform's employment_type enum (the site's own labels in the
// comments). "side_job" (Подработка) is part-time work; anything new is left to the pipeline.
func x5EmploymentType(t string) string {
	switch t {
	case "full_time": // Полная занятость
		return "full_time"
	case "part_time", "side_job": // Частичная занятость, Подработка
		return "part_time"
	case "civil_contract": // Договор ГПХ
		return "contract"
	case "internship": // Стажировка
		return "internship"
	default:
		return ""
	}
}

// x5EducationLevels maps the education picklist. Высшее (higher education) is a degree; the
// rest state that none is required. A value outside the list yields nothing.
var x5EducationLevels = map[string]string{
	"Высшее": "bachelor",
	"Среднее-профессиональное": "none",
	"Среднее":           "none",
	"Не имеет значения": "none",
}

// x5Salary reads the structured salary: an exact figure, or a from/to range. The site prints
// it in roubles with no period, which on the Russian market is a monthly figure.
func x5Salary(it x5Item) (lo, hi *int, currency, period string) {
	if it.Salary != nil {
		lo = roundSalaryPart(*it.Salary)
		hi = lo
	} else {
		if it.SalaryMin != nil {
			lo = roundSalaryPart(*it.SalaryMin)
		}
		if it.SalaryMax != nil {
			hi = roundSalaryPart(*it.SalaryMax)
		}
	}
	if lo == nil && hi == nil {
		return nil, nil, "", ""
	}
	return lo, hi, "RUB", "month"
}

// x5Markdown renders the content fields, which are Markdown written by hand: "* " lists and
// "**bold**" sit beside lines that are plain text one item per line, or "•"-led. Hard wraps
// keep those plain lines apart instead of folding them into one run of text.
var x5Markdown = goldmark.New(goldmark.WithRendererOptions(gmhtml.WithHardWraps()))

// x5Description assembles the body in the site's own layout: key features, the free-text
// description, responsibilities, requirements and conditions, each section only when it has
// something in it. The short picklist-like fields are labelled as the site labels them.
func x5Description(it x5Item, d x5Detail) string {
	c := d.Content
	var md strings.Builder
	section := func(heading string, blocks ...string) {
		var kept []string
		for _, b := range blocks {
			if b = strings.TrimSpace(b); b != "" {
				kept = append(kept, b)
			}
		}
		if len(kept) > 0 {
			md.WriteString("### " + heading + "\n\n" + strings.Join(kept, "\n\n") + "\n\n")
		}
	}
	labelled := func(label, value string) string {
		if value = strings.TrimSpace(value); value == "" {
			return ""
		}
		return "**" + label + ":** " + value
	}
	var features []string
	for _, f := range it.KeyFeatures {
		if f = strings.TrimSpace(f); f != "" {
			features = append(features, "- "+f)
		}
	}
	characterWork := ""
	if c.CharacterWork {
		characterWork = labelled("Характер работы", "Разъездной")
	}

	section("Особенности", strings.Join(features, "\n"))
	section("О вакансии", c.Description)
	section("Обязанности", c.MainResponsibilities)
	section("Требования",
		c.ProfessionalSkills,
		labelled("Образование", c.Education),
		labelled("Опыт работы", c.Experience),
		labelled("Компьютерные навыки", c.SoftwareSkills),
		labelled("Иностранный язык", c.ForeignLanguage),
		labelled("Личные качества", c.PersonalQualities),
		labelled("Дополнительные требования", c.OtherRequirements))
	section("Условия",
		c.WorkingConditions,
		labelled("График работы", it.WorkSchedule),
		labelled("Командировки", c.BusinessTrips),
		characterWork)

	var out strings.Builder
	if err := x5Markdown.Convert([]byte(md.String()), &out); err != nil {
		return ""
	}
	return out.String()
}
