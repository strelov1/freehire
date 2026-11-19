package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// uzum adapts Uzum's careers site (people.uzum.com), a single-company source with no
// per-tenant board id (boardless). Uzum is a holding — Uzum Market, Uzum Bank, Uzum Nasiya,
// Uzum Tezkor — so each vacancy's business unit is its employer, with the configured entry as
// the fallback, the same shape sber has.
//
// Vacancies are enumerated from the public sitemap and read from their own pages. The site
// has a JSON list API (POST /api/v1/vacancy/search, behind a CSRF token), but robots.txt
// disallows /api/ — the fix for a disallowed path is an allowed one, and the sitemap is that.
// It lists every vacancy twice, under /career/ru/ and /career/uz/ with the same id; only the
// Russian copy is read, since the payload's "_en" fields hold the UZBEK text, not English.
//
// A vacancy page is Next.js App Router, and the posting is read from its RSC flight rather than
// its ld+json, because the site publishes a JobPosting block only for office and hybrid roles, so
// every remote vacancy has none. The flight's `vacancy` object is on all of them and is the
// richer record anyway (work mode, experience level and trainee flag as enums).
type uzum struct {
	http uzumHTTP
}

// uzumHTTP is the transport uzum needs: an XML sitemap plus HTML vacancy pages.
type uzumHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	uzumSitemapURL = "https://people.uzum.com/sitemap.xml"
	// uzumCountry is appended to the city: every city the site publishes is in Uzbekistan
	// (its own ld+json states addressCountry "UZ" wherever it emits one), while the location
	// dictionary resolves Tashkent but not Чирчик, Ургенч or Карши on their own.
	uzumCountry = "Узбекистан"
)

// NewUzum builds the Uzum adapter over the given HTTP client.
func NewUzum(c uzumHTTP) Source { return uzum{http: c} }

func (uzum) Provider() string { return "uzum" }

// uzum is single-company, so its config entries carry no board.
func (uzum) Boardless() {}

func (u uzum) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	locs, err := sitemapJobLocs(ctx, u.http, uzumSitemapURL, uzumVacancyID)
	if err != nil {
		return nil, fmt.Errorf("uzum: sitemap: %w", err)
	}
	// A holding with dozens of open roles never has none, so a sitemap naming no vacancy is
	// the URL shape having moved — read as an empty catalogue, it would retire the employer.
	if len(locs) == 0 {
		return nil, fmt.Errorf("uzum: sitemap lists no vacancies — the URL shape changed")
	}

	jobs := fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return u.detail(ctx, e, loc)
	})

	// A run that listed vacancies and read none of them is a failure, not an empty crawl —
	// see AGENTS.md's "a crawl that reads nothing of what it listed must fail".
	if !slices.ContainsFunc(jobs, func(j Job) bool { return !j.Unreadable }) {
		return nil, fmt.Errorf("uzum: listed %d vacancies and read none of them", len(locs))
	}
	return jobs, nil
}

// detail reads one vacancy page. A 404/410 and a vacancy the payload marks withdrawn are drops:
// the status is what the site tells every reader (a sitemap entry can answer 404 while still
// rendering a published payload), so it decides. Anything else that yields no vacancy is an
// unreadableDetail marker, since a whole site answering that way is a markup change, not a mass closure.
func (u uzum) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := uzumVacancyID(loc)
	if id == "" {
		return Job{}, false
	}
	root, err := u.http.GetHTML(ctx, loc)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, loc, e.Company), true
		}
		return Job{}, false
	}
	flight, err := decodeNextFlight(root)
	if err != nil {
		return unreadableDetail(id, loc, e.Company), true
	}
	v, ok := uzumVacancyOf(flight, id)
	if !ok {
		return unreadableDetail(id, loc, e.Company), true
	}
	if v.Disabled || v.Status != "Published" {
		return Job{}, false
	}

	body := v.BodyRU
	if ref, isRef := strings.CutPrefix(body, "$"); isRef {
		body = nextFlightTextRows(flight)[ref]
	}
	workMode := uzumWorkModes[v.WorkMode.Key]

	j := Job{
		ExternalID:         id,
		URL:                loc,
		Title:              firstNonEmpty(strings.TrimSpace(v.TitleRU), strings.TrimSpace(v.TitleEN)),
		Company:            firstNonEmpty(strings.TrimSpace(v.BusinessUnit.Labels.RU), e.Company),
		Location:           joinNonEmpty(strings.TrimSpace(v.City.Labels.RU), uzumCountry),
		Description:        sanitizeHTML(uzumBodyHTML(body)),
		Remote:             workMode == "remote",
		WorkMode:           workMode,
		PostedAt:           parseLayout("01/02/2006", v.PublicationDate),
		ExperienceYearsMin: uzumExperienceYears(v.ExperienceLevel.Key),
	}
	if v.Trainee {
		j.EmploymentType = "internship"
		j.Seniority = "intern"
	}
	return j, true
}

// uzumVacancy is the page's own vacancy object in the RSC flight. body_ru is normally a "$<id>"
// reference to a flight text row. The "_en" fields carry the Uzbek translation.
type uzumVacancy struct {
	ID              int    `json:"id"`
	Status          string `json:"status"`
	Disabled        bool   `json:"disabled"`
	TitleRU         string `json:"title_ru"`
	TitleEN         string `json:"title_en"`
	BodyRU          string `json:"body_ru"`
	PublicationDate string `json:"publication_date"` // MM/DD/YYYY
	Trainee         bool   `json:"trainee"`
	BusinessUnit    struct {
		Labels uzumLabels `json:"labels"`
	} `json:"business_unit"`
	City struct {
		Labels uzumLabels `json:"labels"`
	} `json:"city"`
	WorkMode struct {
		Key string `json:"work_mode_key"`
	} `json:"work_mode"`
	ExperienceLevel struct {
		Key string `json:"experience_level_key"`
	} `json:"experience_level"`
}

type uzumLabels struct {
	RU string `json:"ru"`
}

// uzumVacancyOf decodes the vacancy object whose id is the page's own. Keying on the id rather
// than on the first "vacancy" key matters: the apply form further down the flight names the
// same prop as a "$5:props:…" back-reference string, not an object.
func uzumVacancyOf(flight, id string) (uzumVacancy, bool) {
	raw, ok := bracketSlice(flight, `"vacancy":{"id":`+id+`,`, '{', '}')
	if !ok {
		return uzumVacancy{}, false
	}
	var v uzumVacancy
	if json.Unmarshal([]byte(raw), &v) != nil {
		return uzumVacancy{}, false
	}
	return v, true
}

// uzumBodyHTML renders a vacancy body. The site stores it as its page builder's document,
// {"components":[{"type":"text","value":{"body":"<html>"}}]}, and the text components' HTML is the
// posting. A component of another type costs only itself (hand-authored content, not a crawl
// failure). A body that is not such a document is taken to be the HTML itself.
func uzumBodyHTML(body string) string {
	var doc struct {
		Components []struct {
			Type  string `json:"type"`
			Value struct {
				Body string `json:"body"`
			} `json:"value"`
		} `json:"components"`
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "{") || json.Unmarshal([]byte(body), &doc) != nil {
		return body
	}
	var b strings.Builder
	for _, c := range doc.Components {
		if c.Type == "text" {
			b.WriteString(c.Value.Body)
		}
	}
	return b.String()
}

// uzumWorkModes maps the site's work_mode_key enum onto Job.WorkMode.
var uzumWorkModes = map[string]string{
	"office": "onsite",
	"hybrid": "hybrid",
	"remote": "remote",
}

// uzumExperienceLevels maps the site's experience_level_key enum to the minimum years it states.
var uzumExperienceLevels = map[string]int{
	"no_experience": 0,
	"from_1_year":   1,
	"from_3_years":  3,
	"from_6_years":  6,
}

// uzumExperienceYears returns the minimum years an experience_level_key states, or nil for a
// key it does not recognise.
func uzumExperienceYears(key string) *int {
	years, ok := uzumExperienceLevels[key]
	if !ok {
		return nil
	}
	return &years
}

// uzumVacancyIDPattern captures the id from a Russian vacancy page URL. The /career/uz/ copy
// of the same id is deliberately not matched (see the type doc).
var uzumVacancyIDPattern = regexp.MustCompile(`^https://people\.uzum\.com/career/ru/vacancies/(\d+)/?$`)

// uzumVacancyID extracts the vacancy id from a sitemap loc, or "" when the loc is not a
// Russian vacancy page.
func uzumVacancyID(loc string) string {
	return firstSubmatch(uzumVacancyIDPattern, loc)
}
