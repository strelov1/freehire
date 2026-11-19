package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// tochka adapts Tochka Bank's own careers site (hr.tochka.com), a single-company source with no
// per-tenant board id (boardless). The site is a Next.js App Router app with no public API: the
// listing page server-renders only its first 20 vacancies and fetches the rest through a server
// action, so the vacancies are enumerated from the public sitemap (every /vacancies/catalog/<slug>/
// page) and each one is read from its page:
//
//   - the RSC flight carries the vacancy object — every body section, salary bounds, work
//     format, city, experience band, category and specialization. Long sections arrive as
//     "$N" references into the flight's text rows;
//   - the schema.org JobPosting ld+json states the salary's currency and period and the
//     employment type. Its description is an SEO blurb and its jobLocationType says
//     TELECOMMUTE on every vacancy, office ones included, so neither is read.
//
// The site's TLS certificate chains to the Russian Trusted Root CA, so the adapter is handed
// the client from NewRussianTrustedRootClient rather than the shared one.
type tochka struct {
	http tochkaHTTP
}

// tochkaHTTP is the transport tochka needs: the XML sitemap plus HTML vacancy pages.
type tochkaHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	tochkaSitemapURL = "https://hr.tochka.com/sitemap.xml"
	tochkaVacancyURL = "https://hr.tochka.com/vacancies/catalog/%s/"
)

// NewTochka builds the Tochka Bank adapter over the given HTTP client.
func NewTochka(c tochkaHTTP) Source { return tochka{http: c} }

func (tochka) Provider() string { return "tochka" }

// tochka is single-company, so its config entries carry no board.
func (tochka) Boardless() {}

func (t tochka) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	locs, err := t.vacancyLocs(ctx)
	if err != nil {
		return nil, err
	}
	jobs := fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return t.detail(ctx, e, tochkaSlug(loc))
	})
	// A run that listed vacancies and read none of them is a failure, not an empty crawl —
	// see AGENTS.md's "a crawl that reads nothing of what it listed must fail".
	if len(locs) > 0 && !anyRead(jobs) {
		return nil, fmt.Errorf("tochka: listed %d vacancies and read none of them", len(locs))
	}
	return jobs, nil
}

// vacancyLocs enumerates every vacancy page the sitemap names. Today the sitemap is one flat
// urlset; should it become an index, every sub-sitemap is followed, and one that fails fails
// the run — a partial enumeration would have the sweep close the vacancies it skipped. A
// sitemap naming no vacancy at all is an error too: the site carries dozens, so "none" reads
// the same as "the sitemap moved", and read as the former it would retire the whole employer.
func (t tochka) vacancyLocs(ctx context.Context) ([]string, error) {
	doc, err := getSitemap(ctx, t.http, tochkaSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("tochka: sitemap: %w", err)
	}
	urls := doc.URLs
	for _, sm := range doc.Sitemaps {
		sub, err := getSitemap(ctx, t.http, sm.Loc)
		if err != nil {
			return nil, fmt.Errorf("tochka: sitemap %s: %w", sm.Loc, err)
		}
		urls = append(urls, sub.URLs...)
	}
	var locs []string
	seen := map[string]bool{}
	for _, u := range urls {
		if slug := tochkaSlug(u.Loc); slug != "" && !seen[slug] {
			seen[slug] = true
			locs = append(locs, u.Loc)
		}
	}
	if len(locs) == 0 {
		return nil, fmt.Errorf("tochka: sitemap names no vacancy pages — it moved, or the crawl is being refused")
	}
	return locs, nil
}

// tochkaSlugPattern captures the slug of a vacancy page, /vacancies/catalog/<slug>/. Category
// pages (/vacancies/it/) and the landing pages do not match.
var tochkaSlugPattern = regexp.MustCompile(`^https://hr\.tochka\.com/vacancies/catalog/([^/?#]+)/?$`)

// tochkaSlug returns the vacancy slug of a sitemap URL, or "" when it is not a vacancy page.
// The slug is the vacancy's only identity: the payload carries no numeric id.
func tochkaSlug(loc string) string {
	return firstSubmatch(tochkaSlugPattern, strings.TrimSpace(loc))
}

// tochkaFacet is a category, a specialization (both keyed by slug) or a city (read by name).
type tochkaFacet struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// tochkaVacancy is the vacancy object in the page's flight. The body sections are HTML, or a
// "$N" reference to a text row holding it.
type tochkaVacancy struct {
	Title              string       `json:"title"`
	Slug               string       `json:"slug"`
	Type               string       `json:"type"`
	MainCategory       *tochkaFacet `json:"mainCategory"`
	MainSpecialization *tochkaFacet `json:"mainSpecialization"`
	WorkFormat         string       `json:"workFormat"`
	WorkExperience     string       `json:"workExperience"`
	City               *tochkaFacet `json:"city"`
	PublishedAt        string       `json:"publishedAt"`
	Detail             string       `json:"detail"`
	AboutTeam          string       `json:"aboutTeam"`
	Requirement        string       `json:"requirement"`
	Responsibility     string       `json:"responsibility"`
	Expectation        string       `json:"expectation"`
	BlockTitle         string       `json:"blockTitle"`
	Blocks             []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"blocks"`
}

// tochkaPosting is the slice of the page's JobPosting ld+json read for the salary's unit and
// the employment type; the bounds themselves are the same numbers the vacancy object carries.
type tochkaPosting struct {
	EmploymentType string `json:"employmentType"`
	BaseSalary     struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail reads one vacancy page. A 404/410 (what a closed vacancy answers) drops it; any other
// failure, or a page whose flight holds no vacancy under this slug, yields an unreadable
// marker, since the page is the vacancy's only source.
func (t tochka) detail(ctx context.Context, e CompanyEntry, slug string) (Job, bool) {
	url := fmt.Sprintf(tochkaVacancyURL, slug)
	root, err := t.http.GetHTML(ctx, url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(slug, url, e.Company), true
		}
		return Job{}, false
	}
	flight, err := decodeNextFlight(root)
	if err != nil {
		return unreadableDetail(slug, url, e.Company), true
	}
	v, ok := tochkaVacancyOf(flight, slug)
	if !ok || v.Title == "" {
		return unreadableDetail(slug, url, e.Company), true
	}
	var ld tochkaPosting
	ldJobPosting(root, &ld) // optional: without it the salary carries no unit and is skipped

	job := Job{
		ExternalID:         slug,
		URL:                url,
		Title:              strings.TrimSpace(v.Title),
		Company:            e.Company,
		Description:        sanitizeHTML(tochkaBody(v, nextFlightTextRows(flight))),
		WorkMode:           tochkaWorkMode(v.WorkFormat),
		Seniority:          tochkaSeniority(v.Type),
		Category:           tochkaCategory(v),
		IsTechHint:         v.MainCategory != nil && v.MainCategory.Slug == "it",
		EmploymentType:     schemaEmploymentType(ld.EmploymentType),
		ExperienceYearsMin: tochkaExperienceYears(v.WorkExperience),
		PostedAt:           parseDate(v.PublishedAt),
	}
	if v.City != nil {
		job.Location = v.City.Name
	}
	job.Remote = job.WorkMode == "remote"

	period := strings.ToLower(ld.BaseSalary.Value.UnitText)
	lo, hi := roundSalaryPart(ld.BaseSalary.Value.MinValue), roundSalaryPart(ld.BaseSalary.Value.MaxValue)
	if (lo != nil || hi != nil) && ld.BaseSalary.Currency != "" && isSalaryPeriod(period) {
		job.SalaryMin, job.SalaryMax = lo, hi
		job.SalaryCurrency, job.SalaryPeriod = ld.BaseSalary.Currency, period
	}
	return job, true
}

// tochkaVacancyOf finds the vacancy object whose slug is the page's own. The flight may hold
// other "vacancy" objects too — the "more vacancies" cards render one each — so taking the
// first would read the wrong posting whenever the page's layout put a card first.
func tochkaVacancyOf(flight, slug string) (tochkaVacancy, bool) {
	const key = `"vacancy":`
	for rest := flight; ; {
		at := strings.Index(rest, key)
		if at < 0 {
			return tochkaVacancy{}, false
		}
		rest = rest[at:]
		raw, ok := bracketSlice(rest, key, '{', '}')
		if !ok {
			return tochkaVacancy{}, false
		}
		var v tochkaVacancy
		if json.Unmarshal([]byte(raw), &v) == nil && v.Slug == slug {
			return v, true
		}
		rest = rest[len(key):]
	}
}

// tochkaBody assembles the description in the order and under the headings the page renders
// it. A section held as "$N" is resolved from the flight's text rows; "$$" escapes a literal
// leading dollar.
func tochkaBody(v tochkaVacancy, rows map[string]string) string {
	resolve := func(s string) string {
		if lit, ok := strings.CutPrefix(s, "$$"); ok {
			return "$" + lit
		}
		if ref, ok := strings.CutPrefix(s, "$"); ok {
			return rows[ref]
		}
		return s
	}
	var b strings.Builder
	section := func(heading, body string) {
		if body = strings.TrimSpace(resolve(body)); body == "" {
			return
		}
		if heading != "" {
			b.WriteString("<h3>" + html.EscapeString(heading) + "</h3>")
		}
		b.WriteString(body)
	}
	section("", v.Detail)
	section("О команде", v.AboutTeam)
	section("Что делать", v.Requirement)
	if len(v.Blocks) > 0 {
		var blocks strings.Builder
		for _, bl := range v.Blocks {
			blocks.WriteString("<h4>" + html.EscapeString(bl.Title) + "</h4><p>" + html.EscapeString(bl.Description) + "</p>")
		}
		section(v.BlockTitle, blocks.String())
	}
	section("Ты подойдёшь, если", v.Responsibility)
	section("Что ждёт тебя в Точка Банк", v.Expectation)
	return b.String()
}

// tochkaWorkMode maps the site's workFormat. "trips" (Разъезды) is field sales travelling to
// clients in one city, which is on-site work.
func tochkaWorkMode(f string) string {
	switch f {
	case "remotely":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "office", "trips":
		return "onsite"
	default:
		return ""
	}
}

// tochkaSeniority maps the vacancy's type marker: "lead" is the site's "Руководящая роль".
func tochkaSeniority(t string) string {
	if t == "lead" {
		return "lead"
	}
	return ""
}

// tochkaExperienceYears maps the site's experience band to its lower bound in years, per the
// band's own Russian label.
func tochkaExperienceYears(band string) *int {
	y, ok := tochkaExperienceBands[band]
	if !ok {
		return nil
	}
	return &y
}

var tochkaExperienceBands = map[string]int{
	"up_to_three_years": 1, // От 1 года до 3 лет
	"up_to_five_years":  3, // От 3 до 5 лет
	"over_five_years":   5, // Более 5 лет
}

// tochkaSpecializations and tochkaCategories map the site's own taxonomy where a value names
// one freehire category outright; the specialization is the more precise of the two and wins.
// "it" is deliberately absent — it spans analysts, product managers and engineers, so it sets
// IsTechHint and leaves the subcategory to the title dictionary.
var (
	tochkaSpecializations = map[string]string{
		"data-analytics":       "data_analytics",
		"product-management":   "product",
		"communication-design": "design",
	}
	tochkaCategories = map[string]string{
		"infosec":        "security",
		"design":         "design",
		"sales":          "sales",
		"marketing-i-pr": "marketing",
		"support":        "support",
		"hr":             "hr",
		"finance":        "finance",
		"legal":          "legal",
	}
)

func tochkaCategory(v tochkaVacancy) string {
	if v.MainSpecialization != nil {
		if c, ok := tochkaSpecializations[v.MainSpecialization.Slug]; ok {
			return c
		}
	}
	if v.MainCategory != nil {
		return tochkaCategories[v.MainCategory.Slug]
	}
	return ""
}
