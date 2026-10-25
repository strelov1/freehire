package sources

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// onapply adapts onapply (onapply.de), a German applicant-tracking system for small and mid-sized
// employers. Each customer gets a career site on https://<tenant>.onapply.de, but the vendor also
// runs one job board over ALL of them (jobboard.onapply.de), and that board's Elasticsearch-backed
// API is public and keyless:
//
//	https://api-jobboard.onapply.de/jobboard/production/listing/search/<from>/<size>?jobtitle=&location=
//
// It answers {"total": N, "hits": [{"_id", "_source": {...}}]} and each hit carries the whole
// posting — title, employer, places, employment types and every body section — so one walk of
// the index is the crawl and no posting needs a detail request. The provider is therefore
// boardless and an aggregator: the employer is read per posting (553 employers on 2026-10-25).
//
// See the "onapply traps" section of AGENTS.md for the measurements behind each rule.
type onapply struct {
	http      JSONGetter
	pageSize  int
	maxPasses int
}

// onapplyAPI is the listing endpoint without its <from>/<size> path segments.
const onapplyAPI = "https://api-jobboard.onapply.de/jobboard/production/listing/search"

// onapplyPageSize is the page the walk asks for. The endpoint answers 500 rows (~2 MB) in about a
// second; 5000 in one request is refused with a 502 by the gateway in front of it.
const onapplyPageSize = 500

// onapplyMaxPasses bounds how many times one run walks the index. The search sends no sort, so
// Elasticsearch orders tied scores per shard and a page boundary can repeat one posting and skip
// another: one walk of 4,131 postings yielded 4,037 and 4,082 distinct ids. Two walks unioned
// reached the stated total exactly, so a run walks again while it is short of it.
const onapplyMaxPasses = 4

// NewOnapply builds the adapter over the given HTTP client.
func NewOnapply(c JSONGetter) Source {
	return onapply{http: c, pageSize: onapplyPageSize, maxPasses: onapplyMaxPasses}
}

func (onapply) Provider() string { return "onapply" }

// One vendor-wide index, no per-tenant id.
func (onapply) boardless() {}

// Hundreds of employers post here, each named per posting.
func (onapply) aggregator() {}

type onapplyPage struct {
	Total *int `json:"total"`
	Hits  []struct {
		ID     string         `json:"_id"`
		Source onapplyPosting `json:"_source"`
	} `json:"hits"`
}

// onapplyPosting is one hit's _source. Every body section comes with the heading the employer
// gave it; a heading left blank falls back to the platform's own default.
type onapplyPosting struct {
	ID                    int64    `json:"id"`
	Name                  string   `json:"name"`
	DatetimePublished     string   `json:"datetimePublished"`
	JobAdURL              string   `json:"jobAdUrl"`
	OrganisationName      string   `json:"organisationName"`
	EmploymentTypes       []string `json:"employmentTypes"`
	LocationCountries     []string `json:"locationCountries"`
	LocationRegions       []string `json:"locationRegions"`
	LocationLocalities    []string `json:"locationLocalities"`
	Introduction          string   `json:"jobAdDescriptionIntroduction"`
	IntroductionTitle     string   `json:"jobAdDescriptionIntroductionSectionTitle"`
	Responsibilities      string   `json:"jobAdDescriptionResponsibilities"`
	ResponsibilitiesTitle string   `json:"jobAdDescriptionResponsibilitiesSectionTitle"`
	Qualifications        string   `json:"jobAdDescriptionQualifications"`
	QualificationsTitle   string   `json:"jobAdDescriptionQualificationsSectionTitle"`
	Incentives            string   `json:"jobAdDescriptionIncentives"`
	IncentivesTitle       string   `json:"jobAdDescriptionIncentivesSectionTitle"`
	Contact               string   `json:"jobAdDescriptionContact"`
	ContactTitle          string   `json:"jobAdDescriptionContactSectionTitle"`
}

// Fetch walks the index until it holds as many distinct postings as the index says it has, or
// until maxPasses walks. The first page of the first walk failing fails the crawl; any later page
// failing ends that walk with what was gathered, by the shared paging rule.
func (s onapply) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	seen := make(map[int64]bool)
	var jobs []Job
	total := 0
	for pass := 0; pass < s.maxPasses; pass++ {
		stated, err := s.walk(ctx, pass == 0, func(p onapplyPosting) {
			if p.ID <= 0 || seen[p.ID] {
				return
			}
			seen[p.ID] = true
			if j, ok := onapplyJob(p); ok {
				jobs = append(jobs, j)
			}
		})
		if err != nil {
			return nil, err
		}
		// A repeat walk whose first page failed states no total; the first walk's still holds.
		if stated > 0 {
			total = stated
		}
		if len(seen) >= total {
			break
		}
	}
	return jobs, nil
}

// walk reads the index page by page, handing every hit to visit, and returns the total the index
// states. Only the first page of the first walk may fail the crawl.
func (s onapply) walk(ctx context.Context, first bool, visit func(onapplyPosting)) (int, error) {
	total := 0
	for from := 0; ; from += s.pageSize {
		var page onapplyPage
		err := s.http.GetJSON(ctx, onapplyPageURL(from, s.pageSize), &page)
		if err == nil && page.Total == nil {
			err = fmt.Errorf("answer carries no total")
		}
		if err != nil {
			if from == 0 && first {
				return 0, fmt.Errorf("onapply: listing: %w", err)
			}
			return total, nil
		}
		total = *page.Total
		for _, h := range page.Hits {
			visit(h.Source)
		}
		if len(page.Hits) == 0 || from+s.pageSize >= total {
			return total, nil
		}
	}
}

func onapplyPageURL(from, size int) string {
	return onapplyAPI + "/" + strconv.Itoa(from) + "/" + strconv.Itoa(size) + "?jobtitle=&location="
}

// onapplyJob maps one hit. ok is false for a hit without the title, employer or posting URL a
// stored row needs.
func onapplyJob(p onapplyPosting) (Job, bool) {
	title := strings.TrimSpace(p.Name)
	company := strings.TrimSpace(p.OrganisationName)
	pageURL := onapplyPostingURL(p.JobAdURL)
	if title == "" || company == "" || pageURL == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:     strconv.FormatInt(p.ID, 10),
		URL:            pageURL,
		Title:          title,
		Company:        company,
		Location:       onapplyLocation(p),
		Description:    onapplyDescription(p),
		PostedAt:       onapplyDate(p.DatetimePublished),
		EmploymentType: onapplyEmploymentType(p.EmploymentTypes),
		Countries:      countriesFromCodes(p.LocationCountries),
	}, true
}

// onapplyPostingURL returns the posting's page on its employer's career site. The index stores
// it on the tenant host, but the job board appends a tracking parameter when it hands it out, so
// the query is dropped; a URL that is not https on an onapply host is not a posting page.
func onapplyPostingURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".onapply.de") {
		return ""
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String()
}

// onapplyLocation lists the posting's places. A posting may name dozens of localities (one
// chain named 58); the first few say where it is and the rest add nothing the countries field
// does not already state.
func onapplyLocation(p onapplyPosting) string {
	const maxPlaces = 5
	places := p.LocationLocalities
	if len(places) > maxPlaces {
		places = places[:maxPlaces]
	}
	var region string
	if len(p.LocationRegions) == 1 {
		region = p.LocationRegions[0]
	}
	return joinNonEmpty(distinctJoin(places, "; ", strings.TrimSpace), region)
}

// onapplyDescription assembles the body from the posting's sections in the order the career
// site shows them. The contact section is left out: it names the recruiter and how to reach
// them, which is the posting page's business, not the catalogue's.
func onapplyDescription(p onapplyPosting) string {
	sections := []struct{ title, fallback, body string }{
		{p.IntroductionTitle, "Über uns", p.Introduction},
		{p.ResponsibilitiesTitle, "Aufgaben", p.Responsibilities},
		{p.QualificationsTitle, "Profil", p.Qualifications},
		{p.IncentivesTitle, "Wir bieten", p.Incentives},
	}
	var b strings.Builder
	for _, s := range sections {
		heading := firstNonEmpty(strings.TrimSpace(s.title), s.fallback)
		b.WriteString(headedSection(html.EscapeString(heading), s.body))
	}
	return sanitizeHTML(b.String())
}

// onapplyBerlin is the wall clock datetimePublished is stated in.
var onapplyBerlin = mustLoadLocation("Europe/Berlin")

// onapplyDate reads datetimePublished ("2026-10-23 15:04:54"), a wall-clock time in Germany.
func onapplyDate(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(s), onapplyBerlin)
	if err != nil {
		return nil
	}
	return &t
}

// onapplyEmploymentType maps the posting's employment types. A posting may state several
// ("Vollzeit", "Teilzeit"); only a posting whose types all map to ONE value states it, so
// "Vollzeit" + "Teilzeit" names none. The training types (Ausbildung, Duales Studium,
// Werkstudententätigkeit, Freiwilliges soziales Jahr…) have no counterpart and stay "".
func onapplyEmploymentType(types []string) string {
	out := ""
	for _, t := range types {
		var v string
		switch strings.TrimSpace(t) {
		case "Vollzeit":
			v = "full_time"
		case "Teilzeit", "Minijob / Geringfügig":
			v = "part_time"
		case "Praktikum", "Praxisjahr":
			v = "internship"
		// "Freiberuflliche" is the platform's own spelling.
		case "Freiberuflliche Tätigkeit", "Freiberufliche Tätigkeit":
			v = "contract"
		default:
			return ""
		}
		if out != "" && out != v {
			return ""
		}
		out = v
	}
	return out
}
