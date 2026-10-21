package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // wuzzuf dates are Cairo wall time, and Egypt observes DST

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// wuzzuf adapts WUZZUF (wuzzuf.net), Egypt's largest general job board, whose postings each
// name their own employer — mostly in Egypt, with a minority in Saudi Arabia, the UAE and
// further afield. Board-based aggregator: a board is one of the platform's own job ROLES
// ("IT/Software Development"), which is what keeps a general board on its technical slice —
// 436 of 5,266 active postings on 2026-10-18.
//
// The whole site, its API included, sits behind a Cloudflare challenge, so the crawl reads the
// site's own search pages through the Scrapling tier's browser. Each page embeds the store
// state its front end boots from (Wuzzuf.initialStoreState): that page's slice of the search,
// with each row's posting id and employer, and the full JSON:API job document of every row —
// so one page request per wuzzufPageSize postings carries everything and there is no detail
// fetch. Every document is still checked against the board's role before it is believed: the
// search answers the whole catalogue to a filter it does not understand.
//
// fullBoardListing: the walk reads to a short page and fails unless the distinct ids read
// equal the search's own count, on any failed page, on a count that moves mid-walk and on
// reaching wuzzufMaxPages. See the WUZZUF traps in AGENTS.md.
type wuzzuf struct {
	http TextGetter
	// pageSize is wuzzufPageSize outside tests.
	pageSize int
}

const (
	wuzzufSiteURL       = "https://wuzzuf.net/"
	wuzzufSearchPageURL = wuzzufSiteURL + "search/jobs/"
	// wuzzufPageSize is how many rows the site's search page shows; it is not ours to choose.
	wuzzufPageSize = 15
	// wuzzufMaxPages is a ceiling three times above the IT role's size (437 postings on
	// 2026-10-21), never the expected end: a role whose count does not fit it FAILS rather than
	// being read in part.
	wuzzufMaxPages = 100
)

var (
	// wuzzufBoardRE is a role name as the platform spells it ("IT/Software Development",
	// "Engineering - Telecom/Technology").
	wuzzufBoardRE = regexp.MustCompile(`^[A-Z][A-Za-z&/ -]*[A-Za-z]$`)
	// wuzzufIDRE is a posting id, a UUID.
	wuzzufIDRE = regexp.MustCompile(`^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)
	// wuzzufURIRE is a posting page's path on wuzzuf.net: jobs/p/<slug>, internship/<slug>,
	// saudi/jobs/p/<slug>.
	wuzzufURIRE = regexp.MustCompile(`^(?:[a-z]+/)+[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// wuzzufPageInterval paces the search pages. On 2026-10-21 nine pages ~6 s apart were served
// and the tenth answered 429; at one per 12 s a run of ~30 pages takes ~6 minutes.
const wuzzufPageInterval = 12 * time.Second

// pacedWuzzufSite paces the search-page requests of one run.
func pacedWuzzufSite(c TextGetter) TextGetter {
	return rateLimitedTextGetter{inner: c, limiter: rate.NewLimiter(rate.Every(wuzzufPageInterval), 1)}
}

// wuzzufCairo is the zone the API's zone-less posting dates are written in.
var wuzzufCairo = mustLoadLocation("Africa/Cairo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// NewWuzzuf builds the WUZZUF adapter over the given HTTP client (the Scrapling tier's browser
// in production).
func NewWuzzuf(c TextGetter) Source {
	return wuzzuf{http: c, pageSize: wuzzufPageSize}
}

func (wuzzuf) Provider() string { return "wuzzuf" }

// Every posting names its own employer, so wuzzuf stays in the source facet and in the
// cross-source duplicate suppression set.
func (wuzzuf) aggregator() {}

// fullBoardListing: Fetch walks the role's search to a short page and fails unless the ids
// read equal the search's count, all distinct, and every document is of the board's role. See
// the fullBoardListing interface (source.go) for the bar.
func (wuzzuf) fullBoardListing() {}

// wuzzufPageState is the slice of a search page's store state the crawl reads: the search
// result, keyed by the page's own path, and the job documents of its rows.
type wuzzufPageState struct {
	JobSearch struct {
		Sets map[string]wuzzufSearchSet `json:"sets"`
	} `json:"jobSearch"`
	Entities struct {
		Job struct {
			Collection map[string]wuzzufDoc `json:"collection"`
		} `json:"job"`
	} `json:"entities"`
}

// wuzzufSearchSet is one page of the search: the rows in order, the rows by id and the count.
type wuzzufSearchSet struct {
	TotalResultsCount *int                 `json:"totalResultsCount"`
	ResultsOrder      []string             `json:"resultsOrder"`
	ResultsSet        map[string]wuzzufHit `json:"resultsSet"`
}

// wuzzufStateRE finds where a page assigns its store state.
var wuzzufStateRE = regexp.MustCompile(`Wuzzuf\.initialStoreState\s*=\s*`)

// wuzzufHit is one search row: the posting id and, among its computed fields, the employer.
type wuzzufHit struct {
	ID         string `json:"id"`
	Attributes struct {
		ComputedFields []struct {
			Name  string `json:"name"`
			Value []any  `json:"value"`
		} `json:"computedFields"`
	} `json:"attributes"`
}

// company is the employer the search names, "" when the employer chose to hide it.
func (h wuzzufHit) company() string {
	for _, f := range h.Attributes.ComputedFields {
		if f.Name == "company_name" && len(f.Value) > 0 {
			s, _ := f.Value[0].(string)
			return strings.Join(strings.Fields(s), " ")
		}
	}
	return ""
}

type wuzzufNamed struct {
	Name string `json:"name"`
}

// wuzzufDoc is one job document as the API states it. Nullable objects decode to nil.
type wuzzufDoc struct {
	ID         string `json:"id"`
	Attributes struct {
		Status       string `json:"status"`
		JobType      string `json:"jobType"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Requirements string `json:"requirements"`
		URI          string `json:"uri"`
		PostedAt     string `json:"postedAt"`
		HideSalary   bool   `json:"hideSalary"`
		Salary       struct {
			Min      *float64 `json:"min"`
			Max      *float64 `json:"max"`
			IsPaid   bool     `json:"isPaid"`
			Currency *struct {
				Code string `json:"code"`
			} `json:"currency"`
			Period *wuzzufNamed `json:"period"`
		} `json:"salary"`
		Keywords             []wuzzufNamed `json:"keywords"`
		CareerLevel          *wuzzufNamed  `json:"careerLevel"`
		WorkplaceArrangement *struct {
			DisplayedName string `json:"displayedName"`
		} `json:"workplaceArrangement"`
		Location struct {
			Country *struct {
				Name string `json:"name"`
				Code string `json:"code"`
			} `json:"country"`
			City *wuzzufNamed `json:"city"`
			Area *wuzzufNamed `json:"area"`
		} `json:"location"`
		WorkExperienceYears struct {
			Min *int `json:"min"`
		} `json:"workExperienceYears"`
		WorkRoles []wuzzufNamed `json:"workRoles"`
		WorkTypes []wuzzufNamed `json:"workTypes"`
	} `json:"attributes"`
}

func (s wuzzuf) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if !wuzzufBoardRE.MatchString(e.Board) {
		return nil, fmt.Errorf("wuzzuf: board %q is not a role name", e.Board)
	}
	hits, docs, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, h := range hits {
		company := h.company()
		if company == "" {
			continue // the employer is hidden, so the posting names no company
		}
		d, ok := docs[h.ID]
		if !ok {
			// The page listed the posting without its document: no answer about it, not the
			// platform saying it is gone.
			jobs = append(jobs, unreadableDetail(h.ID, "", company))
			continue
		}
		if !d.hasRole(e.Board) {
			return nil, fmt.Errorf("wuzzuf: board %q: posting %s is not of the role, the search ignored its filter",
				e.Board, h.ID)
		}
		if job, ok := d.toJob(company); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// list walks the role's search pages to their natural end, failing on any page error, on a
// count that moves mid-walk, on a repeated id and on a total the walk did not read in full. It
// returns the rows in the search's order and the job documents the pages carried, by id.
func (s wuzzuf) list(ctx context.Context, role string) ([]wuzzufHit, map[string]wuzzufDoc, error) {
	var hits []wuzzufHit
	docs := map[string]wuzzufDoc{}
	seen := map[string]bool{}
	total := -1
	for page := 0; ; page++ {
		if page == wuzzufMaxPages {
			return nil, nil, fmt.Errorf("wuzzuf: board %q: reached %d pages without the end", role, wuzzufMaxPages)
		}
		set, err := s.page(ctx, role, page, docs)
		if err != nil {
			return nil, nil, fmt.Errorf("wuzzuf: board %q: search page %d: %w", role, page, err)
		}
		count := *set.TotalResultsCount
		switch {
		case page > 0 && count != total:
			return nil, nil, fmt.Errorf("wuzzuf: board %q: the count moved from %d to %d mid-walk", role, total, count)
		case count == 0:
			// The platform's roles always carry postings; nothing at all is a misspelled role
			// (an unknown value matches nothing), not a board whose postings all closed.
			return nil, nil, fmt.Errorf("wuzzuf: board %q matches no postings", role)
		case count > wuzzufMaxPages*s.pageSize:
			return nil, nil, fmt.Errorf("wuzzuf: board %q lists %d postings, over the %d the walk reads",
				role, count, wuzzufMaxPages*s.pageSize)
		}
		total = count
		for _, id := range set.ResultsOrder {
			h, ok := set.ResultsSet[id]
			if !ok || !wuzzufIDRE.MatchString(id) || h.ID != id {
				return nil, nil, fmt.Errorf("wuzzuf: board %q: unreadable posting id %q", role, id)
			}
			if seen[id] {
				return nil, nil, fmt.Errorf("wuzzuf: board %q: posting %s listed twice", role, id)
			}
			seen[id] = true
			hits = append(hits, h)
		}
		if len(set.ResultsOrder) < s.pageSize {
			break
		}
	}
	if len(hits) != total {
		return nil, nil, fmt.Errorf("wuzzuf: board %q: read %d of %d postings", role, len(hits), total)
	}
	return hits, docs, nil
}

// page reads one search page's store state, adding the page's job documents to docs. The
// returned set always states a count.
func (s wuzzuf) page(ctx context.Context, role string, page int, docs map[string]wuzzufDoc) (wuzzufSearchSet, error) {
	u := wuzzufSearchPageURL + "?filters%5Broles%5D%5B0%5D=" +
		strings.ReplaceAll(url.QueryEscape(role), "+", "%20") + "&start=" + strconv.Itoa(page)
	body, err := s.http.GetText(ctx, u)
	if err != nil {
		return wuzzufSearchSet{}, err
	}
	loc := wuzzufStateRE.FindStringIndex(body)
	if loc == nil {
		return wuzzufSearchSet{}, fmt.Errorf("the page carries no store state")
	}
	var st wuzzufPageState
	if err := json.NewDecoder(strings.NewReader(body[loc[1]:])).Decode(&st); err != nil {
		return wuzzufSearchSet{}, fmt.Errorf("store state: %w", err)
	}
	if len(st.JobSearch.Sets) != 1 {
		return wuzzufSearchSet{}, fmt.Errorf("the store state holds %d searches, want 1", len(st.JobSearch.Sets))
	}
	var set wuzzufSearchSet
	for _, only := range st.JobSearch.Sets {
		set = only
	}
	if set.TotalResultsCount == nil {
		return wuzzufSearchSet{}, fmt.Errorf("the search states no count")
	}
	maps.Copy(docs, st.Entities.Job.Collection)
	return set, nil
}

func (d wuzzufDoc) hasRole(role string) bool {
	for _, r := range d.Attributes.WorkRoles {
		if r.Name == role {
			return true
		}
	}
	return false
}

func (d wuzzufDoc) toJob(company string) (Job, bool) {
	a := d.Attributes
	title := strings.Join(strings.Fields(a.Title), " ")
	if a.Status != "active" || title == "" || !wuzzufURIRE.MatchString(a.URI) {
		return Job{}, false
	}
	body := a.Description
	if strings.TrimSpace(a.Requirements) != "" {
		body += "<h3>Requirements</h3>" + a.Requirements
	}
	job := Job{
		ExternalID:     d.ID,
		URL:            wuzzufSiteURL + a.URI,
		Title:          title,
		Company:        company,
		Description:    sanitizeHTML(body),
		PostedAt:       wuzzufPostedAt(a.PostedAt),
		EmploymentType: d.employmentType(),
		Seniority:      d.seniority(),
		IsTechHint:     true, // every board is a role, and the crawl reads the IT one
	}
	if a.WorkExperienceYears.Min != nil && *a.WorkExperienceYears.Min > 0 {
		years := *a.WorkExperienceYears.Min
		job.ExperienceYearsMin = &years
	}
	if w := a.WorkplaceArrangement; w != nil {
		job.WorkMode = wuzzufWorkModes[w.DisplayedName]
	}
	job.Remote = job.WorkMode == "remote"
	loc := a.Location
	var place []string
	for _, n := range []*wuzzufNamed{loc.Area, loc.City} {
		if n != nil {
			place = append(place, n.Name)
		}
	}
	if loc.Country != nil {
		place = append(place, loc.Country.Name)
		if c := location.NormalizeCountry(strings.TrimSpace(loc.Country.Code)); c != "" {
			job.Countries = []string{c}
		}
	}
	job.Location = distinctJoin(place, ", ", strings.TrimSpace)
	skills := make([]string, 0, len(a.Keywords))
	for _, k := range a.Keywords {
		skills = append(skills, k.Name)
	}
	job.Skills = skilltag.Canonicalize(skills)
	d.salary(&job)
	return job, true
}

// wuzzufPostedAt reads the API's "MM/DD/YYYY HH:MM:SS", which is Cairo wall time: a posting
// read at 18:00 UTC carried 20:19 as its posting time.
func wuzzufPostedAt(s string) *time.Time {
	t, err := time.ParseInLocation("01/02/2006 15:04:05", strings.TrimSpace(s), wuzzufCairo)
	if err != nil {
		return nil
	}
	return NotFuture(&t)
}

var wuzzufWorkModes = map[string]string{
	"On-site": "onsite",
	"Hybrid":  "hybrid",
	"Remote":  "remote",
}

// wuzzufEmploymentTypes maps the platform's work types onto vocab.EmploymentTypeValues.
// shift_based has no counterpart.
var wuzzufEmploymentTypes = map[string]string{
	"full_time":         "full_time",
	"part_time":         "part_time",
	"freelance_project": "contract",
	"internship":        "internship",
}

// employmentType is stated only when the posting names exactly one work type: 14 of 436 IT
// postings name several ("full_time, part_time, freelance_project"), which says nothing.
func (d wuzzufDoc) employmentType() string {
	if len(d.Attributes.WorkTypes) != 1 {
		return ""
	}
	return wuzzufEmploymentTypes[d.Attributes.WorkTypes[0].Name]
}

// seniority maps the career levels that name one: an internship or a student is an intern,
// "Entry Level" a junior. "Experienced" spans middle and senior and "Manager" / "Senior
// Management" a management track, so they are left to the title dictionary.
func (d wuzzufDoc) seniority() string {
	a := d.Attributes
	if a.JobType == "internship" {
		return "intern"
	}
	if a.CareerLevel == nil {
		return ""
	}
	switch a.CareerLevel.Name {
	case "Student":
		return "intern"
	case "Entry Level":
		return "junior"
	default:
		return ""
	}
}

var wuzzufSalaryPeriods = map[string]string{
	"Per Hour":  "hour",
	"Per Day":   "day",
	"Per Month": "month",
	"Per Year":  "year",
}

// salary takes the posting's range only when the employer shows it and it is paid: a hidden
// salary is withheld by the employer, and an unpaid internship carries a 0-0 range.
func (d wuzzufDoc) salary(job *Job) {
	sal := d.Attributes.Salary
	if d.Attributes.HideSalary || !sal.IsPaid || sal.Currency == nil || sal.Period == nil {
		return
	}
	period := wuzzufSalaryPeriods[sal.Period.Name]
	currency := strings.ToUpper(strings.TrimSpace(sal.Currency.Code))
	if period == "" || len(currency) != 3 {
		return
	}
	var lo, hi *int
	if sal.Min != nil {
		lo = roundSalaryPart(*sal.Min)
	}
	if sal.Max != nil {
		hi = roundSalaryPart(*sal.Max)
	}
	if lo == nil && hi == nil {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, period
}
