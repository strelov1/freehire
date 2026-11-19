package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// kaspi adapts Kaspi.kz's career site (job.kaspi.kz): a single-company, server-rendered Nuxt
// site with no per-tenant board id (boardless). Vacancies are enumerated from the vacancy
// sitemap and each page carries a schema.org JobPosting ld+json block — the avito shape.
//
// The site also answers a paginated JSON search API, but robots.txt disallows /api/web/, so
// the sitemap is the source of record.
//
// The ld+json is thin and partly filler: addressLocality is the country code "KZ",
// addressRegion is the company name, and employmentType is the literal string " Array ". So
// title, body, date and country come from it, while the city, salary, schedule and experience
// come from the server-rendered page header, which is where the site itself shows them.
type kaspi struct {
	http kaspiHTTP
}

// kaspiHTTP is the transport kaspi needs: the XML sitemap plus HTML vacancy pages.
type kaspiHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	kaspiSitemapURL = "https://job.kaspi.kz/sitemap/vacancies.xml"
	// kaspiDateLayout is datePosted's layout. It carries no zone, and it is UTC: it equals
	// the sitemap's lastmod (stated with "Z") to the millisecond.
	kaspiDateLayout = "2006-01-02 15:04:05.999999"

	kaspiCitiesClass      = "vacancy-single__head__content__salary__cities"
	kaspiSalaryClass      = "vacancy-single__head__content__salary__label"
	kaspiFilterLabelClass = "vacancy-single__head__content__filter-items__label"
)

// NewKaspi builds the Kaspi.kz adapter over the given HTTP client.
func NewKaspi(c kaspiHTTP) Source { return kaspi{http: c} }

func (kaspi) Provider() string { return "kaspi" }

// kaspi is single-company, so its config entries carry no board.
func (kaspi) Boardless() {}

func (k kaspi) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	locs, err := sitemapJobLocs(ctx, k.http, kaspiSitemapURL, kaspiVacancyID)
	if err != nil {
		return nil, fmt.Errorf("kaspi: sitemap: %w", err)
	}
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return k.detail(ctx, e, loc)
	}), nil
}

// kaspiPosting is the part of the JobPosting ld+json worth reading (see the type comment).
type kaspiPosting struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
	JobLocation struct {
		Address struct {
			AddressCountry string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// detail fetches one vacancy page and maps it to a Job. The page is the posting's only
// source, so a page the platform reports gone is dropped while a page this crawl merely
// could not read comes back as an unreadableDetail marker.
func (k kaspi) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := kaspiVacancyID(loc)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := k.http.GetHTML(ctx, loc)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, loc, e.Company), true
		}
		return Job{}, false
	}
	var p kaspiPosting
	if !ldJobPosting(root, &p) {
		return unreadableDetail(id, loc, e.Company), true
	}

	location := kaspiLocation(kaspiClassText(root, kaspiCitiesClass))
	labels := kaspiClassTexts(root, kaspiFilterLabelClass)
	salaryMin, salaryMax, currency, period := kaspiSalary(kaspiClassText(root, kaspiSalaryClass))

	return Job{
		ExternalID: id,
		URL:        loc,
		Title:      p.Title,
		// The configured company is canonical; hiringOrganization only repeats "Kaspi.kz".
		Company:  e.Company,
		Location: location,
		// The body is entity-escaped HTML inside the JSON string, as on avito.
		Description:        sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:             isRemote(location) || isRemote(p.Title),
		Countries:          countryFromCode(p.JobLocation.Address.AddressCountry),
		EmploymentType:     kaspiEmploymentType(labels),
		ExperienceYearsMin: kaspiExperienceFloor(labels),
		SalaryMin:          salaryMin,
		SalaryMax:          salaryMax,
		SalaryCurrency:     currency,
		SalaryPeriod:       period,
		PostedAt:           parseLayout(kaspiDateLayout, p.DatePosted),
	}, true
}

// kaspiVacancyIDPattern captures the slug from a /vacancy/<slug> page URL. The slug is the
// only identifier the page states, and the apply form under it (/vacancy/<slug>/respond) is
// not a vacancy.
var kaspiVacancyIDPattern = regexp.MustCompile(`^https://job\.kaspi\.kz/vacancy/([^/]+)/?$`)

// kaspiVacancyID returns the vacancy slug of a page URL, or "" for any other URL.
func kaspiVacancyID(loc string) string {
	return firstSubmatch(kaspiVacancyIDPattern, trimURLSuffix(loc))
}

// kaspiClassText returns the whitespace-collapsed text of the first element with the class.
func kaspiClassText(root *html.Node, class string) string {
	n := firstByClass(root, class)
	if n == nil {
		return ""
	}
	return strings.Join(strings.Fields(textContent(n)), " ")
}

// kaspiClassTexts returns the trimmed text of every element with the class, in page order.
func kaspiClassTexts(root *html.Node, class string) []string {
	var out []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && HasClass(n, class) {
			out = append(out, textContent(n))
			return false
		}
		return true
	})
	return out
}

// kaspiOfficeNote is the "(Головной офис)" — head office — note the city line ends with when
// the role sits there. It names no place.
var kaspiOfficeNote = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// kaspiLocation is the page's city line ("Алматы, Астана" — a vacancy may list several)
// without its trailing office note.
func kaspiLocation(cities string) string {
	return kaspiOfficeNote.ReplaceAllString(cities, "")
}

// kaspiEmploymentType reads the schedule labels. Only "Полный день" (full day) states the
// employment type; "Гибкий график" and "Сменный график" are schedules and say nothing of it.
func kaspiEmploymentType(labels []string) string {
	for _, l := range labels {
		if l == "Полный день" {
			return "full_time"
		}
	}
	return ""
}

// kaspiExperienceYears maps the site's experience picklist (hh.ru's bands) to the band's floor.
var kaspiExperienceYears = map[string]int{
	"Нет опыта":          0,
	"От 1 года до 3 лет": 1,
	"От 2 до 4 лет":      2,
	"От 3 до 6 лет":      3,
	"Более 6 лет":        6,
}

// kaspiExperienceFloor is the lowest band a vacancy is tagged with — a vacancy may carry
// several, and the lowest is what it accepts. nil when it carries none.
func kaspiExperienceFloor(labels []string) *int {
	var floor *int
	for _, l := range labels {
		if years, ok := kaspiExperienceYears[l]; ok && (floor == nil || years < *floor) {
			floor = &years
		}
	}
	return floor
}

// kaspiSalaryPattern matches the whole salary label: an optional "от"/"до", one amount with
// spaces or dots as thousands separators, and an optional tenge sign. Anything else — a
// range, a word, another currency — yields nothing rather than a guess.
var kaspiSalaryPattern = regexp.MustCompile(`(?i)^(от|до)?\s*(\d[\d\s.]*\d|\d)\s*(тенге|тг|₸)?$`)

// kaspiSalary reads the salary label of the page header. The site only ever states tenge, with
// or without the word, per month (Kazakhstan's convention): "от 800 000 тенге" is a floor,
// "до ..." a ceiling, and a bare "300 000" an exact figure.
func kaspiSalary(label string) (lo, hi *int, currency, period string) {
	m := kaspiSalaryPattern.FindStringSubmatch(strings.TrimSpace(label))
	if m == nil {
		return nil, nil, "", ""
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, m[2])
	amount, err := strconv.Atoi(digits)
	if err != nil || amount <= 0 {
		return nil, nil, "", ""
	}
	switch strings.ToLower(m[1]) {
	case "от":
		lo = &amount
	case "до":
		hi = &amount
	default:
		lo, hi = &amount, &amount
	}
	return lo, hi, "KZT", "month"
}
