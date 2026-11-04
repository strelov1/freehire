package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// dassault adapts Dassault Systèmes' careers catalogue (www.3ds.com/careers/jobs), a
// single-company source with no per-tenant board id (boardless). The requisitions live in
// Taleo (talentacquisition.3ds.com, careersection "qa"), but that section is not public: its
// search page redirects to www.3ds.com/careers and a job detail answers 302, so the `taleo`
// adapter cannot read it ("no portalNo"). What 3ds.com's own job search calls instead is an
// Exalead "card search" API, whose career cards carry every posting WITH its full body — so a
// crawl is a handful of paged GETs and no detail request.
//
// Measured live 2026-10-15: 697 postings. Each exists once per site language (8,364 cards over
// 12 languages), and the English refinement alone covers all 697 distinct requisitions.
type dassault struct {
	http XMLGetter
}

const (
	// dassaultSearchURL lists the English career cards. Both refinements are load-bearing:
	// without the content-type facet the index is 3ds.com's whole card corpus, and without the
	// language facet every posting comes back once per language. `hf` is the page size.
	dassaultSearchURL = "https://www.3ds.com/apisearch/card_search_api?lang=en" +
		"&r=f/card_content_type/career" +
		"&r=f/card_content_categories_facet/cards%%20language/en" +
		"&hf=" + dassaultPageSizeStr + "&start=%d"
	dassaultPageSizeStr = "100"
	dassaultPageSize    = 100
	// dassaultMaxPages bounds the walk; reaching it before the stated total fails the crawl.
	dassaultMaxPages = 100
)

// NewDassault builds the Dassault Systèmes careers adapter over the given HTTP client.
func NewDassault(c XMLGetter) Source { return dassault{http: c} }

func (dassault) Provider() string { return "dassault" }

// dassault is single-company, so its config entry carries no board.
func (dassault) boardless() {}

// fullBoardListing: every page states the hit count (`nhits`), and Fetch fails unless the
// distinct postings read equal it. Any page failing — a later one included — fails the crawl.
// The answer's `estimated` attribute is NOT the proof: at hf=100 it read "true" on 23 of 24
// requests while nhits stayed 697 and the walk read exactly 697 distinct cards, so it says
// nothing usable about the count; the count check is what carries the claim.
func (dassault) fullBoardListing() {}

// dassaultAnswer is the Exalead response (served as application/javascript, but XML).
type dassaultAnswer struct {
	NHits int           `xml:"nhits,attr"`
	Hits  []dassaultHit `xml:"hits>Hit"`
}

type dassaultHit struct {
	Metas []struct {
		Name   string   `xml:"name,attr"`
		Values []string `xml:"MetaString"`
	} `xml:"metas>Meta"`
}

// meta returns the first value of the named meta field, or "".
func (h dassaultHit) meta(name string) string {
	for _, m := range h.Metas {
		if m.Name == name && len(m.Values) > 0 {
			return strings.TrimSpace(m.Values[0])
		}
	}
	return ""
}

func (s dassault) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var (
		jobs []Job
		seen = map[string]bool{}
	)
	for page := 0; page < dassaultMaxPages; page++ {
		var resp dassaultAnswer
		if err := s.http.GetXML(ctx, fmt.Sprintf(dassaultSearchURL, page*dassaultPageSize), &resp); err != nil {
			return nil, fmt.Errorf("dassault: list page %d: %w", page, err)
		}
		for _, h := range resp.Hits {
			id := h.meta("card_id")
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if job, ok := dassaultJob(h, id, e); ok {
				jobs = append(jobs, job)
			}
		}
		if len(resp.Hits) < dassaultPageSize {
			if len(seen) != resp.NHits {
				return nil, fmt.Errorf("dassault: read %d distinct postings, the index states %d", len(seen), resp.NHits)
			}
			return jobs, nil
		}
	}
	return nil, fmt.Errorf("dassault: no end of the listing after %d pages", dassaultMaxPages)
}

// dassaultCategories splits content_categories, a flat "Label/value" string whose values hold
// spaces and commas. All 697 postings carried the same six labels in the same order, so the
// split anchors on that order rather than guessing where a value ends.
var dassaultCategories = regexp.MustCompile(`^Category/(.*) Type/(.*) Country/(.*) City/(.*) Products/(.*) Year/(.*)$`)

// dassaultYears reads the lower bound of the "Experience level" facet ("4 to 5 years", "More
// than 15 years"); "Not specified" yields nothing.
var dassaultYears = regexp.MustCompile(`^(?:More than )?(\d+)\b`)

// dassaultJob maps one card. The card id is the Taleo requisition number, the same id the
// posting page's URL ends in and its "Ref ID" states.
func dassaultJob(h dassaultHit, id string, e CompanyEntry) (Job, bool) {
	title := strings.Join(strings.Fields(h.meta("content_title")), " ")
	link := h.meta("content_cta_1_url")
	desc := sanitizeHTML(h.meta("content_summary"))
	if u, err := url.Parse(link); err != nil || u.Host != "www.3ds.com" {
		return Job{}, false
	}
	if title == "" || strings.TrimSpace(desc) == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID:  id,
		URL:         link,
		Title:       title,
		Company:     e.Company,
		Description: desc,
		Location:    h.meta("content_info_2_value"),
		PostedAt:    dassaultDate(h.meta("content_start_datetime")),
	}
	m := dassaultCategories.FindStringSubmatch(h.meta("content_categories"))
	if m == nil {
		return job, true
	}
	jobType, country, city, years := strings.TrimSpace(m[2]), dassaultCountry(m[3]), strings.TrimSpace(m[4]), strings.TrimSpace(m[6])
	job.Location = dassaultLocation(city, country)
	job.Countries = countryFromCode(country)
	switch jobType {
	case "Internship":
		job.EmploymentType, job.Seniority = "internship", "intern"
	case "Fixed Term Contract":
		job.EmploymentType = "contract"
	}
	if ym := dassaultYears.FindStringSubmatch(years); ym != nil {
		// Parsed at 8 bits: the facet's bounds are 0-15, and the value is stored as an int32.
		if n, err := strconv.ParseUint(ym[1], 10, 8); err == nil {
			years := int(n)
			job.ExperienceYearsMin = &years
		}
	}
	return job, true
}

// dassaultCountry returns the Country facet in a form the country dictionary reads. Two of the
// site's names carry a qualifier after a comma that it does not ("Korea, Republic of";
// "Taiwan,China", which the location parser would even read as TWO countries), so a name that
// does not resolve whole falls back to its first part when that part does.
func dassaultCountry(raw string) string {
	name := strings.TrimSpace(raw)
	if countryFromCode(name) != nil {
		return name
	}
	if head, _, ok := strings.Cut(name, ","); ok && countryFromCode(head) != nil {
		return strings.TrimSpace(head)
	}
	return name
}

// dassaultLocation renders "<city>, <country>". The City facet repeats the country and often a
// region code before the city ("India, MH, Pune", "Korea, Republic of, 11, Seoul"), so the city
// is its last comma-separated part.
func dassaultLocation(city, country string) string {
	if i := strings.LastIndex(city, ","); i >= 0 {
		city = strings.TrimSpace(city[i+1:])
	}
	switch {
	case city == "" || strings.EqualFold(city, country):
		return country
	case country == "":
		return city
	}
	return city + ", " + country
}

// dassaultDate reads content_start_datetime ("2026/10/14 03:10:27"), the date the posting page
// shows as "Posted on".
func dassaultDate(s string) *time.Time {
	return parseLayout("2006/01/02 15:04:05", strings.TrimSpace(s))
}
