package sources

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// megafon adapts MegaFon's careers API (job.megafon.ru), a single-company source with no
// per-tenant board id (boardless). MegaFon publishes for several entities (МегаФон, МегаФон
// Ритейл, МегаТех, ЕдиныйФактор), so each posting's own organisation is the employer.
//
// The listing carries no body, which lives behind a per-posting detail request, so the adapter
// hydrates (HydratingSource): a detail request is spent only on a posting the catalogue lacks.
//
// The listing is walked CITY BY CITY, not as one feed, and that is the load-bearing decision.
// The unfiltered feed pages 10 at a time ordered by publication time alone, and hundreds of
// retail postings share one timestamp, so their order is not stable from page to page: a full
// walk returns duplicates and always misses the same postings, which the unseen sweep would close
// for good. No sort parameter is honoured and the page size is capped at 10. Filtered to one city
// a listing is a page or two, small enough that the instability never shows. It costs one request
// per city the site knows (most a single empty-or-short page), the price of a complete listing.
type megafon struct {
	http JSONGetter
}

const (
	megafonCitiesURL = "https://job.megafon.ru/api/v1/cities/available"
	// megafonListURL is one city's listing page; page counts from 1.
	megafonListURL = "https://job.megafon.ru/api/v1/vacancies?cities=%d&page=%d"
	// megafonDetailURL answers only with the city the posting is listed in.
	megafonDetailURL = "https://job.megafon.ru/api/v1/vacancies/%s?cityId=%d"
	// megafonJobURL is the public posting page: /vacancy/{city}/{sector}/{slug}.
	megafonJobURL = "https://job.megafon.ru/vacancy/%s/%s/%s"
	// megafonMaxPages is a safety ceiling on one city's walk: reaching it before the city's
	// own page count fails the crawl.
	megafonMaxPages = 100
	// megafonCityWorkers bounds the city walk's in-flight requests.
	megafonCityWorkers = 4
	// megafonNationwideCities is the number of cities past which a posting is stated as Russia-wide
	// rather than as a list: the site's two standing retail ads are each listed in some 600 cities,
	// while a genuinely multi-city role names a handful.
	megafonNationwideCities = 25
)

// The site's two pseudo-cities: a remote listing and an "all cities" one.
const (
	megafonRemoteCitySlug = "udalennyj-format"
	megafonAllCitiesSlug  = "vse-goroda"
	megafonITSectorSlug   = "it-reshenia"
)

// NewMegafon builds the MegaFon adapter over the given HTTP client.
func NewMegafon(c JSONGetter) Source { return megafon{http: c} }

func (megafon) Provider() string { return "megafon" }

// megafon is single-company, so its config entry carries no board.
func (megafon) Boardless() {}

// megafonCity is a city as both the cities endpoint and a listed posting state it.
type megafonCity struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// megafonListed is one listed posting. Its city is the city the listing was filtered to.
type megafonListed struct {
	ID           int         `json:"id"`
	Slug         string      `json:"slug"`
	Title        string      `json:"title"`
	City         megafonCity `json:"city"`
	Organisation struct {
		Title string `json:"title"`
	} `json:"organisation"`
	Sector *struct {
		Slug string `json:"slug"`
	} `json:"sector"`
	Specialties []string `json:"specialties"`
	PublishedAt int64    `json:"publishedAt"`
	Tag         *struct {
		Code string `json:"code"`
	} `json:"tag"`
}

// megafonPosting is a listed posting with every city it was listed in, in the order the
// cities endpoint ranks them (remote and Moscow first).
type megafonPosting struct {
	megafonListed
	cities []megafonCity
}

// megafonDetail is the per-posting payload: the body in three HTML sections plus the facts the
// listing omits. The salary bounds are usually null but are read when present.
type megafonDetail struct {
	Description  string   `json:"description"`
	Requirements string   `json:"requirements"`
	Conditions   string   `json:"conditions"`
	Experience   string   `json:"experience"`
	MinSalary    *float64 `json:"minSalary"`
	MaxSalary    *float64 `json:"maxSalary"`
	KeySkills    []struct {
		Title string `json:"title"`
	} `json:"keySkills"`
}

// Fetch hydrates every listed posting. FetchNew is the path ingest drives.
func (m megafon) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return m.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the listing city by city and fetches a posting's detail only when the
// catalogue does not already hold it. A seen posting yields a liveness-only refresh; a failed
// detail is logged and the posting ingested list-only, so it is never lost to it.
func (m megafon) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := m.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p megafonPosting) (Job, bool) {
		base := p.toJob(e)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		d, err := m.fetchDetail(ctx, p)
		if err != nil {
			log.Printf("megafon: detail %d failed; ingesting list-only: %v", p.ID, err)
			return base, true
		}
		return d.apply(base), true
	}), nil
}

// crawl reads the city list, walks every city's listing, and merges the postings by id. Any
// listing page failing fails the crawl: that city's postings would otherwise read as unseen.
func (m megafon) crawl(ctx context.Context) ([]megafonPosting, error) {
	var cities struct {
		Cities []megafonCity `json:"cities"`
	}
	if err := m.http.GetJSON(ctx, megafonCitiesURL, &cities); err != nil {
		return nil, fmt.Errorf("megafon: cities: %w", err)
	}

	perCity := make([][]megafonListed, len(cities.Cities))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(megafonCityWorkers)
	for i, c := range cities.Cities {
		g.Go(func() error {
			listed, err := m.listCity(gctx, c.ID)
			perCity[i] = listed
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge in city order, so a posting's first city is the highest-ranked one it is listed in.
	var postings []megafonPosting
	index := map[int]int{}
	for _, listed := range perCity {
		for _, l := range listed {
			if l.ID == 0 || strings.TrimSpace(l.Title) == "" {
				continue
			}
			if i, ok := index[l.ID]; ok {
				postings[i].cities = append(postings[i].cities, l.City)
				continue
			}
			index[l.ID] = len(postings)
			postings = append(postings, megafonPosting{megafonListed: l, cities: []megafonCity{l.City}})
		}
	}
	return postings, nil
}

// listCity pages one city's listing until its page count or an empty page. Reaching the page
// cap first is a failure, not a result: the postings past it would read as unseen.
func (m megafon) listCity(ctx context.Context, cityID int) ([]megafonListed, error) {
	var all []megafonListed
	for page := 1; ; page++ {
		if page > megafonMaxPages {
			return nil, fmt.Errorf("megafon: city %d: page cap %d reached before the end of the listing", cityID, megafonMaxPages)
		}
		var resp struct {
			Pages     int             `json:"pages"`
			Vacancies []megafonListed `json:"vacancies"`
		}
		if err := m.http.GetJSON(ctx, fmt.Sprintf(megafonListURL, cityID, page), &resp); err != nil {
			return nil, fmt.Errorf("megafon: city %d page %d: %w", cityID, page, err)
		}
		all = append(all, resp.Vacancies...)
		if len(resp.Vacancies) == 0 || page >= resp.Pages {
			return all, nil
		}
	}
}

// fetchDetail fetches a posting's detail under the first city it is listed in.
func (m megafon) fetchDetail(ctx context.Context, p megafonPosting) (megafonDetail, error) {
	var d megafonDetail
	if err := m.http.GetJSON(ctx, fmt.Sprintf(megafonDetailURL, p.Slug, p.cities[0].ID), &d); err != nil {
		return megafonDetail{}, err
	}
	return d, nil
}

// toJob maps the listing's view of a posting to a Job — everything but the body and the
// facts only the detail carries.
func (p megafonPosting) toJob(e CompanyEntry) Job {
	sector := ""
	if p.Sector != nil {
		sector = p.Sector.Slug
	}
	job := Job{
		ExternalID: strconv.Itoa(p.ID),
		URL:        fmt.Sprintf(megafonJobURL, p.cities[0].Slug, sector, p.Slug),
		Title:      p.Title,
		Company:    firstNonEmpty(p.Organisation.Title, e.Company),
		PostedAt:   parseEpochSeconds(p.PublishedAt),
		Category:   megafonCategory(p.Specialties),
		IsTechHint: sector == megafonITSectorSlug,
	}

	var places []string
	remoteListed, nationwide := false, len(p.cities) > megafonNationwideCities
	for _, c := range p.cities {
		switch c.Slug {
		case megafonRemoteCitySlug:
			remoteListed = true
		case megafonAllCitiesSlug:
			nationwide = true
		default:
			places = append(places, c.Title)
		}
	}
	if nationwide {
		job.Location = "Россия"
		job.Countries = []string{"ru"}
	} else {
		job.Location = distinctJoin(places, "; ", func(s string) string { return s })
	}

	tag := ""
	if p.Tag != nil {
		tag = p.Tag.Code
	}
	switch {
	case tag == "hybrid_shift":
		job.WorkMode = "hybrid"
	case tag == "remote" || remoteListed:
		job.WorkMode = "remote"
	}
	job.Remote = job.WorkMode == "remote"
	if tag == "no_experience" {
		zero := 0
		job.ExperienceYearsMin = &zero
	}
	return job
}

// apply adds the detail's body and facts to a list-derived job.
func (d megafonDetail) apply(base Job) Job {
	base.Description = sanitizeHTML(
		headedSection("Обязанности", d.Description) +
			headedSection("Требования", d.Requirements) +
			headedSection("Условия", d.Conditions))
	if years := megafonExperienceYears(d.Experience); years != nil {
		base.ExperienceYearsMin = years
	}
	names := make([]string, 0, len(d.KeySkills))
	for _, s := range d.KeySkills {
		names = append(names, s.Title)
	}
	base.Skills = skilltag.Canonicalize(names)
	base.SalaryMin, base.SalaryMax, base.SalaryCurrency, base.SalaryPeriod = rubMonthlySalary(d.MinSalary, d.MaxSalary)
	return base
}

// megafonExperience maps the site's experience picklist to its lower bound in years.
var megafonExperience = map[string]int{
	"без опыта":     0,
	"до 1 года":     0,
	"1-3 года":      1,
	"от 3 до 6 лет": 3,
	"более 6 лет":   6,
}

// megafonExperienceYears returns the lower bound of an experience value, or nil for one the
// picklist does not hold.
func megafonExperienceYears(s string) *int {
	years, ok := megafonExperience[strings.ToLower(strings.TrimSpace(s))]
	if !ok {
		return nil
	}
	return &years
}

// megafonSpecialtyCategories maps the specialties that pin exactly ONE freehire category. The
// rest are deliberately absent: "DevOps/SRE" names two, "Системный анализ" and "Аналитика" are
// business-side roles as often as data ones, and the network specialties are telecom field
// engineering. An unmapped specialty leaves the category to the title dictionary.
var megafonSpecialtyCategories = map[string]string{
	"backend": "backend",
	"мобильная разработка":        "mobile",
	"тестирование":                "qa",
	"информационная безопасность": "security",
	"архитектура":                 "architecture",
}

// megafonCategory resolves a posting's specialties to a category only when every mapped one
// agrees — two specialties naming different categories state neither.
func megafonCategory(specialties []string) string {
	category := ""
	for _, s := range specialties {
		c, ok := megafonSpecialtyCategories[strings.ToLower(strings.TrimSpace(s))]
		if !ok {
			continue
		}
		if category != "" && category != c {
			return ""
		}
		category = c
	}
	return category
}
