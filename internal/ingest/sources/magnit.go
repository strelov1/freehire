package sources

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// magnit adapts Magnit Tech's careers API (magnit.tech), the IT arm of the Magnit retail group:
// a single-company source with no per-tenant board id (boardless). The listing carries every
// structured field but no description, which lives behind a per-posting detail request, so the
// adapter hydrates (HydratingSource): a detail is spent only on a posting the catalogue does not
// already hold. The whole catalogue fits one listing page at per_page=50.
//
// The API refuses curl's default User-Agent with a 403 and answers the project's own; the
// SPA's HTML routes do the same, which is why nothing here reads a page.
type magnit struct {
	http JSONGetter
}

const (
	magnitListURL   = "https://magnit.tech/api/v1/vacancy?page=%d&per_page=50"
	magnitDetailURL = "https://magnit.tech/api/v1/vacancy/%d"
	magnitJobURL    = "https://magnit.tech/vacancies/%d"
	// magnitMaxPages bounds the walk as a backstop; has_more_pages is the primary stop.
	magnitMaxPages = 20
)

// NewMagnit builds the Magnit Tech adapter over the given HTTP client.
func NewMagnit(c JSONGetter) Source { return magnit{http: c} }

func (magnit) Provider() string { return "magnit" }

// magnit is single-company, so its config entries carry no board.
func (magnit) boardless() {}

// magnitNamed is the {name} shape every dictionary-backed field of the API shares.
type magnitNamed struct {
	Name string `json:"name"`
}

// magnitItem is one listed posting. location is null on most postings; the places live in
// the work-format labels instead ("Москва (гибрид/офис)").
type magnitItem struct {
	ID           int           `json:"id"`
	Title        string        `json:"title"`
	Location     string        `json:"location"`
	IsInternship bool          `json:"is_internship"`
	Speciality   *magnitNamed  `json:"speciality"`
	WorkFormats  []magnitNamed `json:"work_formats"`
	Technologies []magnitNamed `json:"technologies"`
}

// magnitDetail is the per-posting payload: the body split into sections (HTML, except
// about_product, which the site stores as plain text), plus the live status.
type magnitDetail struct {
	Description  string `json:"description"`
	AboutProduct string `json:"about_product"`
	Tasks        string `json:"tasks"`
	Skills       string `json:"skills"`
	ExtraSkills  string `json:"extra_skills"`
	Offer        string `json:"offer"`
	ForBeginners bool   `json:"for_beginners"`
	IsPublished  bool   `json:"is_published"`
	Status       string `json:"status"`
}

// Fetch is the list-only crawl (no description) — the fallback for callers that do not drive
// hydration. FetchNew is the hydrating path used by ingest.
func (m magnit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := m.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, it := range items {
		if job, ok := it.toJob(e); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew pages the same listing but fetches a posting's detail only when the catalogue does
// not already hold it. A seen posting yields a liveness-only refresh; a posting the API answers
// 404 for, or reports closed, is gone and dropped; any other detail failure keeps the posting
// list-only, since the catalogue re-offers a body-less row for hydration on later crawls.
func (m magnit) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	items, err := m.list(ctx)
	if err != nil {
		return nil, err
	}
	var attempted, failed, refreshed atomic.Int64
	jobs := fetchDetails(items, defaultDetailWorkers, func(it magnitItem) (Job, bool) {
		base, ok := it.toJob(e)
		if !ok {
			return Job{}, false
		}
		if seen(base.ExternalID) {
			refreshed.Add(1)
			base.SeenRefresh = true
			return base, true
		}
		attempted.Add(1)
		var resp struct {
			Results magnitDetail `json:"results"`
		}
		if err := m.http.GetJSON(ctx, fmt.Sprintf(magnitDetailURL, it.ID), &resp); err != nil {
			if !detailUnreadable(err) {
				return Job{}, false // the platform's own answer that the posting is gone
			}
			failed.Add(1)
			log.Printf("magnit: detail %d failed; ingesting list-only: %v", it.ID, err)
			return base, true
		}
		return resp.Results.apply(base)
	})
	// A run that listed postings and read none of them is a board failure, not an empty crawl
	// (internal/ingest/sources/AGENTS.md, "A crawl that reads nothing of what it listed must
	// fail"): every detail failing with nothing seen is an outage, not a quiet source.
	if n := attempted.Load(); n > 0 && failed.Load() == n && refreshed.Load() == 0 {
		return nil, fmt.Errorf("magnit: listed %d postings and read none of them", len(items))
	}
	return jobs, nil
}

// list walks the listing until the API reports no further page. Any page failing fails the
// crawl; the first page is the board.
func (m magnit) list(ctx context.Context) ([]magnitItem, error) {
	var items []magnitItem
	for page := 1; page <= magnitMaxPages; page++ {
		var resp struct {
			Meta struct {
				HasMorePages bool `json:"has_more_pages"`
			} `json:"meta"`
			Results []magnitItem `json:"results"`
		}
		if err := m.http.GetJSON(ctx, fmt.Sprintf(magnitListURL, page), &resp); err != nil {
			return nil, fmt.Errorf("magnit: list page %d: %w", page, err)
		}
		items = append(items, resp.Results...)
		if !resp.Meta.HasMorePages || len(resp.Results) == 0 {
			return items, nil
		}
	}
	return nil, fmt.Errorf("magnit: listing still reports more pages after %d", magnitMaxPages)
}

// toJob maps a listed posting to a Job, returning ok=false for one with no id to key on or no
// title to name it.
func (it magnitItem) toJob(e CompanyEntry) (Job, bool) {
	if it.ID == 0 || strings.TrimSpace(it.Title) == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID: strconv.Itoa(it.ID),
		URL:        fmt.Sprintf(magnitJobURL, it.ID),
		Title:      it.Title,
		Company:    e.Company,
		// The work-format labels are where the source states its places ("Москва и Краснодар
		// (гибрид/офис), другие города - удаленно"); location adds a country when it is set.
		Location: distinctJoin(append([]magnitNamed{{Name: it.Location}}, it.WorkFormats...), "; ",
			func(n magnitNamed) string { return n.Name }),
		WorkMode: magnitWorkMode(it.WorkFormats),
		Skills:   skilltag.Canonicalize(magnitNames(it.Technologies)),
		// magnit.tech is the IT subsidiary's own careers site and every speciality in its
		// taxonomy is an IT one; retail hiring lives on rabota.magnit.ru, which is not crawled.
		IsTechHint: true,
	}
	if it.Speciality != nil {
		job.Category = magnitCategories[strings.ToLower(strings.TrimSpace(it.Speciality.Name))]
	}
	if it.IsInternship {
		job.EmploymentType = "internship"
	}
	return job, true
}

// apply enriches a list-derived job with its detail, returning ok=false for a posting the
// detail reports unpublished or no longer active: the listing serves live postings only, but
// one can close between the two requests.
func (d magnitDetail) apply(base Job) (Job, bool) {
	if !d.IsPublished || !strings.EqualFold(strings.TrimSpace(d.Status), "active") {
		return Job{}, false
	}
	base.Description = sanitizeHTML(magnitBody(d))
	if d.ForBeginners {
		base.Seniority = "junior"
	}
	return base, true
}

// magnitBody assembles the sectioned body in the order the site renders it. Each section's
// heading is the employer's own wording, and an empty section contributes nothing — not even
// its heading.
func magnitBody(d magnitDetail) string {
	var b strings.Builder
	b.WriteString(d.Description)
	for _, s := range []struct{ heading, body string }{
		{"О продукте", d.AboutProduct},
		{"Задачи", d.Tasks},
		{"Требования", d.Skills},
		{"Будет плюсом", d.ExtraSkills},
		{"Мы предлагаем", d.Offer},
	} {
		body := strings.TrimSpace(s.body)
		if body == "" {
			continue
		}
		if !strings.Contains(body, "<") {
			body = plainTextToHTML(body)
		}
		b.WriteString("<h3>" + s.heading + "</h3>" + body)
	}
	return b.String()
}

// magnitWorkMode maps the work-format labels to a work mode. The vocabulary is small and
// employer-curated and every label says its arrangement in words: "удаленно" (remote), "гибрид"
// (hybrid), "офис" (on-site). A label offering more than one ("Москва и Краснодар (гибрид/офис),
// другие города - удаленно") takes the broadest arrangement, as in workModeFromRemoteHybrid, so a
// candidate filtering for remote work can take this job.
func magnitWorkMode(formats []magnitNamed) string {
	mode := ""
	rank := map[string]int{"": 0, "onsite": 1, "hybrid": 2, "remote": 3}
	for _, f := range formats {
		l := strings.ToLower(f.Name)
		var m string
		switch {
		case strings.Contains(l, "удал"):
			m = "remote"
		case strings.Contains(l, "гибрид"):
			m = "hybrid"
		case strings.Contains(l, "офис"):
			m = "onsite"
		}
		if rank[m] > rank[mode] {
			mode = m
		}
	}
	return mode
}

// magnitCategories maps the specialities that pin exactly ONE freehire category. The rest are
// deliberately absent: "Данные и аналитика" spans data engineering, science and analytics,
// "Управление продуктами и проектами" spans product and project management, "Системный и бизнес
// - анализ" names two different analyst roles, and "IT-инфраструктура" and "1C" pin no single
// discipline. An unmapped speciality leaves Category empty so the title dictionary decides.
var magnitCategories = map[string]string{
	"backend":     "backend",
	"fullstack":   "fullstack",
	"qa":          "qa",
	"ml&ai":       "ml_ai",
	"дизайн":      "design",
	"архитектура": "architecture",
	"информационная безопасность": "security",
}

// magnitNames lists the names of a dictionary-backed field.
func magnitNames(items []magnitNamed) []string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return names
}
