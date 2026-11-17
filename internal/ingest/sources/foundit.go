package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// foundit adapts foundit.in (formerly Monster India), a general Indian job board whose postings
// each name their own employer. It is a boardless aggregator that crawls the board's IT
// SEARCH SLICES: the catalogue is ~210k postings of every trade, the search serves at most its
// newest 10,000 rows per query, and its structured categories are unusable (77% of postings are
// filed under the industry "Other"; the designation "Software Engineer/Programmer" holds sales
// managers). A slice is therefore a keyword query over the last 30 days; the slices
// overlap and are deduplicated by posting id within the run, and no
// tech hint is set (the site has no reliable technical signal).
//
// The listing is the JSON search the site's own web app calls (/middleware/jobsearch); a row
// carries no body, so this is a HydratingSource that reads a posting's body from the detail
// endpoint (/middleware/jobdetail/<id>) only when the catalogue lacks it. Both endpoints answer
// a plain client "content negotiation failed" (HTTP 400) unless the request carries a foundit
// Referer, and the site refuses a plain Go client's TLS fingerprint, so production reads them
// through the Scrapling tier (scraplingtier.go). See the foundit traps in AGENTS.md.
type foundit struct {
	http HeaderJSONGetter
	// spent counts the detail requests claimed across the run.
	spent *atomic.Int64
}

const (
	founditBase      = "https://www.foundit.in"
	founditSearchURL = founditBase + "/middleware/jobsearch?"
	founditDetailURL = founditBase + "/middleware/jobdetail/%s"

	// founditPageSize is the most rows a search page serves (limit=100 answered 100, plus ad rows).
	founditPageSize = 100
	// founditWindow is the search's deepest row: start+limit past 10,000 repeats the last page
	// instead of answering an error, so the walk must stop at it.
	founditWindow = 10_000
	// founditSortDate is sort=2, strictly newest first by createdAt; the default (1) is a
	// relevance order that interleaves days.
	founditSortDate = "2"
	// founditFreshnessDays is the jobFreshness window. A posting lives ~45 days (the listing's
	// closedAt on a sampled row was 44.7 days after createdAt), so the 30-day window re-lists a
	// live posting for most of its life and founditSweepGrace covers the rest.
	founditFreshnessDays = 30
	// founditSweepGrace absorbs the window: a live posting older than the freshness window is
	// not listed, and the detail endpoint is not a liveness probe the sweep can ask. 30 + 21
	// days is past the ~45 days a posting lives, so one is closed after it expired.
	founditSweepGrace = 21 * 24 * time.Hour

	// Pacing: every request of a run, search and detail alike, shares one limiter. Measured
	// 2026-11-17 through the Scrapling tier: a detail ~0.55 s, a 30-row search ~0.6 s.
	founditRequestInterval = 500 * time.Millisecond
	founditRequestBurst    = 2
	founditDetailWorkers   = 4
	// founditDetailBudget bounds one run's detail requests (~12.5 minutes at the pace above);
	// the backlog accretes over later runs and steady state is a few hundred new postings a run.
	founditDetailBudget = 1500
)

// founditSlices are the keyword queries one crawl walks. Each held under 10,000 postings in the
// 30-day window on 2026-11-17 (the broadest, "sap", 4,566), so none is truncated. They are
// full-text queries over the body as well as the title, hence noisy; the catalogue keeps
// non-technical postings, so the noise is stored and the slices only decide what is reachable.
var founditSlices = []string{
	"software engineer", "software developer", "developer", "programmer", "full stack",
	"backend", "frontend", "java", ".net", "react", "android", "ios", "devops", "qa",
	"data engineer", "data scientist", "machine learning", "sap", "salesforce", "embedded",
	"network",
}

// founditHeaders: without a foundit Referer the API answers HTTP 400 "content negotiation
// failed"; any foundit URL is accepted.
var founditHeaders = map[string]string{"Referer": founditBase + "/"}

// NewFoundit builds the foundit adapter over the given HTTP client. The registry builds one
// adapter per process and cmd/ingest crawls one provider per process, so the adapter's detail
// counter is the run's.
func NewFoundit(c HeaderJSONGetter) Source {
	return foundit{http: c, spent: new(atomic.Int64)}
}

// pacedFounditGetter holds every request of a run to one limiter.
func pacedFounditGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(founditRequestInterval), founditRequestBurst),
	}
}

func (foundit) Provider() string { return "foundit" }

// boardless: one crawl walks the slices above; there is no per-tenant board.
func (foundit) boardless() {}

// Every posting names its own employer, so foundit stays in the source facet and in the
// cross-source duplicate suppression set.
func (foundit) aggregator() {}

func (foundit) sweepGrace() time.Duration { return founditSweepGrace }

// founditListing is one search row, reduced to what the adapter reads. Ad rows carry a Type
// ("adsense", "banner") and no id.
type founditListing struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Title             string `json:"title"`
	CompanyName       string `json:"companyName"`
	HideCompanyName   int    `json:"hideCompanyName"`
	Locations         string `json:"locations"`
	SeoJdURL          string `json:"seoJdUrl"`
	CreatedAt         int64  `json:"createdAt"`
	IsJobActive       *bool  `json:"isJobActive"`
	HideSalary        int    `json:"hideSalary"`
	SalaryConfidental bool   `json:"jobSalaryConfidential"`
	MinimumExperience struct {
		Years int `json:"years"`
	} `json:"minimumExperience"`
	MinimumSalary   founditMoney `json:"minimumSalary"`
	MaximumSalary   founditMoney `json:"maximumSalary"`
	EmploymentTypes []string     `json:"employmentTypes"`
	JobTypes        []string     `json:"jobTypes"`
}

type founditMoney struct {
	Currency      string `json:"currency"`
	AbsoluteValue int64  `json:"absoluteValue"`
}

type founditSearchPage struct {
	Status   int `json:"jobSearchStatus"`
	Response struct {
		Data []founditListing `json:"data"`
		Meta struct {
			Paging struct {
				Total int `json:"total"`
			} `json:"paging"`
		} `json:"meta"`
	} `json:"jobSearchResponse"`
}

// founditDetail is the detail endpoint, reduced to what the search row lacks. The endpoint
// answers HTTP 200 whatever the posting's state; the state is jobDetailStatus (404 for an
// unknown or removed id).
type founditDetail struct {
	Status   int `json:"jobDetailStatus"`
	Response struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Locations   []struct {
			City    string `json:"city"`
			State   string `json:"state"`
			Country string `json:"country"`
		} `json:"locations"`
		JobTypes        []string `json:"jobTypes"`
		EmploymentTypes []string `json:"employmentTypes"`
	} `json:"jobDetailResponse"`
}

func (s foundit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s foundit) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated is FetchNew that buys no body for a posting the pipeline will discard anyway: a
// discarded posting is never stored, so without the gate newest-first hydration would buy the
// same covered postings every run (the naukri failure of 2026-10-25).
func (s foundit) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, rows, seen, gate)
}

// list walks every slice's freshness window newest first and returns the postings once each,
// newest first, so the detail budget buys the newest. A slice whose first page fails is logged
// and skipped, a later page failing ends that slice with what was read; the crawl fails when
// no slice answered at all, since "nothing listed" must not pass for a quiet source.
func (s foundit) list(ctx context.Context) ([]founditListing, error) {
	var out []founditListing
	ids := map[string]bool{}
	var firstErr error
	answered := 0
	for _, query := range founditSlices {
		rows, err := s.walk(ctx, query)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			log.Printf("foundit: slice %q failed: %v", query, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		answered++
		for _, r := range rows {
			if ids[r.ID] {
				continue
			}
			ids[r.ID] = true
			out = append(out, r)
		}
	}
	if answered == 0 {
		return nil, firstErr
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// walk reads one slice to its declared total, an empty page, or the search's window. A slice
// past the window is read to it and logged: the window is a property of the platform.
func (s foundit) walk(ctx context.Context, query string) ([]founditListing, error) {
	var out []founditListing
	for start := 0; start < founditWindow; start += founditPageSize {
		q := url.Values{}
		q.Set("query", query)
		q.Set("start", strconv.Itoa(start))
		q.Set("limit", strconv.Itoa(founditPageSize))
		q.Set("sort", founditSortDate)
		q.Set("jobFreshness", strconv.Itoa(founditFreshnessDays))
		var p founditSearchPage
		err := s.http.GetJSONWithHeaders(ctx, founditSearchURL+q.Encode(), founditHeaders, &p)
		if err == nil && p.Status != http.StatusOK {
			err = fmt.Errorf("search answered status %d", p.Status)
		}
		if err != nil {
			return founditPageFailed(query, start, out, err)
		}
		jobs := 0
		for _, r := range p.Response.Data {
			if r.Type != "" || r.ID == "" {
				continue // an ad slot
			}
			jobs++
			out = append(out, r)
		}
		total := p.Response.Meta.Paging.Total
		if jobs == 0 || start+founditPageSize >= total {
			break
		}
		if start+2*founditPageSize > founditWindow {
			log.Printf("foundit: slice %q: %d postings in the window, the search serves the newest %d", query, total, founditWindow)
		}
	}
	return out, nil
}

// founditPageFailed applies the walk's rule: the first page failing fails the slice, a later
// page ends it with what was read.
func founditPageFailed(query string, start int, read []founditListing, err error) ([]founditListing, error) {
	if start == 0 {
		return nil, fmt.Errorf("foundit: slice %q: %w", query, err)
	}
	log.Printf("foundit: slice %q: page at %d failed; keeping %d postings read: %v", query, start, len(read), err)
	return read, nil
}

// hydrate maps the listed rows to Jobs. A seen posting is a liveness refresh with no request,
// a posting whose employer is already covered is yielded body-less, and every other posting costs one detail
// request up to the run's budget. A detail that could not be read, or that the budget did not
// reach, defers its posting to a later run rather than storing it body-less, which would never
// be hydrated again.
func (s foundit) hydrate(ctx context.Context, rows []founditListing, seen func(string) bool, gate DetailGate) ([]Job, error) {
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(rows))
		for _, r := range rows {
			companies = append(companies, strings.TrimSpace(r.CompanyName))
		}
		covered = gate.Covered(companies)
	}
	var jobs, claimed []Job
	deferred := 0
	for _, r := range rows {
		base, ok := r.toJob()
		if !ok {
			continue
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] {
			jobs = append(jobs, base)
			continue
		}
		if s.spent.Add(1) > founditDetailBudget {
			deferred++
			continue
		}
		claimed = append(claimed, base)
	}
	var answered atomic.Int64
	jobs = append(jobs, fetchDetails(claimed, founditDetailWorkers, func(base Job) (Job, bool) {
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			log.Printf("foundit: detail %s failed; deferring: %v", base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1) // a posting the platform reports gone is an answer too
		return job, ok
	})...)
	// A crawl that asked for details and got no answer to any of them is broken, not quiet.
	if len(claimed) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("foundit: requested %d posting details and none answered", len(claimed))
	}
	if deferred > 0 {
		log.Printf("foundit: detail budget reached (%d a run); %d new postings deferred to later runs", founditDetailBudget, deferred)
	}
	return jobs, nil
}

// withDetail adds the detail endpoint's body, structured places, and work arrangement and
// employment type. ok=false with a nil error is the platform's own answer that the posting is
// gone; an error is the absence of an answer.
func (s foundit) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	var d founditDetail
	if err := s.http.GetJSONWithHeaders(ctx, fmt.Sprintf(founditDetailURL, base.ExternalID), founditHeaders, &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	switch d.Status {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone:
		return Job{}, false, nil
	default:
		return Job{}, false, fmt.Errorf("detail answered status %d", d.Status)
	}
	r := d.Response
	if r.ID != base.ExternalID {
		return Job{}, false, fmt.Errorf("posting answered id %q", r.ID)
	}
	body := sanitizeHTML(r.Description)
	if strings.TrimSpace(body) == "" {
		return Job{}, false, fmt.Errorf("posting carries no body")
	}
	base.Description = body
	// A place naming only a country is the posting's country again when another place names a
	// city (live: [{Noida, Uttar Pradesh, India}, {India}]), so it adds nothing beside it.
	hasCity := false
	for _, l := range r.Locations {
		hasCity = hasCity || strings.TrimSpace(l.City) != ""
	}
	var places []string
	seenPlace := map[string]bool{}
	for _, l := range r.Locations {
		if hasCity && strings.TrimSpace(l.City) == "" {
			continue
		}
		p := joinNonEmpty(strings.TrimSpace(l.City), strings.TrimSpace(l.State), strings.TrimSpace(l.Country))
		if p != "" && !seenPlace[p] {
			seenPlace[p] = true
			places = append(places, p)
		}
	}
	if len(places) > 0 {
		base.Location = strings.Join(places, "; ")
	}
	if base.EmploymentType == "" {
		base.EmploymentType = founditEmploymentType(r.JobTypes, r.EmploymentTypes)
	}
	if founditHasType(r.JobTypes, "work from home") {
		base.WorkMode, base.Remote = "remote", true
	}
	return base, true, nil
}

// toJob maps a search row to a body-less Job. The row's comma-separated places stand in until
// the detail's structured ones replace them ("Bengaluru, India" is a city and a country,
// "Bengaluru, Chennai, Hyderabad" three cities, so the row cannot be split reliably).
func (r founditListing) toJob() (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	company := strings.TrimSpace(r.CompanyName)
	path := strings.TrimSpace(r.SeoJdURL)
	if r.ID == "" || title == "" || company == "" || r.HideCompanyName != 0 || !strings.HasPrefix(path, "/job/") {
		return Job{}, false
	}
	if r.IsJobActive != nil && !*r.IsJobActive {
		return Job{}, false
	}
	j := Job{
		ExternalID:        r.ID,
		URL:               founditBase + path,
		Title:             title,
		Company:           company,
		Location:          strings.TrimSpace(r.Locations),
		FallbackCountries: []string{"in"},
		EmploymentType:    founditEmploymentType(r.JobTypes, r.EmploymentTypes),
	}
	if founditHasType(r.JobTypes, "work from home") {
		j.WorkMode, j.Remote = "remote", true
	}
	if r.CreatedAt > 0 {
		t := time.UnixMilli(r.CreatedAt)
		j.PostedAt = NotFuture(&t)
	}
	if n := r.MinimumExperience.Years; n > 0 {
		j.ExperienceYearsMin = &n
	}
	// absoluteValue is the annual figure whatever the display; a hidden or confidential one
	// is not published.
	lo, hi := r.MinimumSalary, r.MaximumSalary
	if r.HideSalary == 0 && !r.SalaryConfidental && lo.AbsoluteValue > 0 && hi.AbsoluteValue >= lo.AbsoluteValue {
		min, max := int(lo.AbsoluteValue), int(hi.AbsoluteValue)
		j.SalaryMin, j.SalaryMax = &min, &max
		j.SalaryCurrency = strings.ToUpper(firstNonEmpty(strings.TrimSpace(lo.Currency), "INR"))
		j.SalaryPeriod = "year"
	}
	return j, true
}

// founditHasType reports whether a posting's job-type list names want (case-insensitive).
func founditHasType(types []string, want string) bool {
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), want) {
			return true
		}
	}
	return false
}

// founditEmploymentType maps the job types ("Contract Job", "Internship", "Permanent Job") and
// the schedule ("Full time", "Part time"). A contract or internship type wins over the
// schedule, which is "Full time" for 99% of postings whatever their nature.
func founditEmploymentType(jobTypes, schedules []string) string {
	for _, t := range jobTypes {
		t = strings.ToLower(strings.TrimSpace(t))
		switch {
		case strings.Contains(t, "intern"):
			return "internship"
		case strings.Contains(t, "contract"):
			return "contract"
		}
	}
	for _, s := range schedules {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "full time":
			return "full_time"
		case "part time":
			return "part_time"
		}
	}
	return ""
}
