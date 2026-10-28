package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// freework adapts Free-Work (www.free-work.com), France's largest IT-only job board: freelance
// missions (~55% of the board states a day rate) beside permanent and fixed-term roles, posted
// mostly by IT consultancies and staffing agencies. It is boardless and an aggregator — the
// employer is read per posting. See the "Free-Work traps" section of AGENTS.md for the
// measurements behind each rule.
//
// The listing is the site's own public API (API Platform, asked for plain JSON), walked
// with a property filter that keeps a 300-posting page at ~120 KB; with every field a page of 50
// takes ~15 s to render, so the whole board in one shape would outlast a run. The body comes from
// the posting's own resource, bought only for postings the catalogue lacks, within a per-run
// budget; the rest are refreshed by identity.
type freework struct {
	get            JSONGetter
	limiter        waiter
	maxNewPerRun   int
	maxPages       int
	deadlineMargin time.Duration
}

const (
	freeworkAPI = "https://api.free-work.com"
	// freeworkListURL selects only what the gate and a body-less Job need. A nested property
	// filter works for the embedded company but not for the job (it is an IRI there), which is
	// why the page URL is completed from the posting resource.
	freeworkListURL = freeworkAPI + "/job_postings?page=%d&itemsPerPage=%d" +
		"&properties%%5B%%5D=id&properties%%5B%%5D=slug&properties%%5B%%5D=title" +
		"&properties%%5B%%5D=publishedAt&properties%%5B%%5D=remoteMode" +
		"&properties%%5Bcompany%%5D%%5B%%5D=name" +
		"&properties%%5Blocation%%5D%%5B%%5D=label&properties%%5Blocation%%5D%%5B%%5D=countryCode"
	freeworkDetailURL = freeworkAPI + "/job_postings/%s"
	freeworkSite      = "https://www.free-work.com/fr/tech-it/job-mission/"
	freeworkPageSize  = 300

	// freeworkRequestInterval paces every request of a run at one a second — a courtesy ceiling,
	// not a measured limit (no refusal was drawn at that pace).
	freeworkRequestInterval = time.Second
	freeworkDetailWorkers   = 2
	// freeworkMaxPages is a safety ceiling: the board held 11,642 postings (39 pages) on
	// 2026-10-28.
	freeworkMaxPages = 200
	// freeworkMaxNewPerRun bounds the posting requests of one run: ~40 listing requests (~4 s
	// each) and 1,500 posting requests at one a second, plus the writes that follow, sit inside
	// the scheduler's default 2,400 s crawl budget. ~400 postings are created a day, so after the
	// first ~8 runs drain the board a run reads a few dozen.
	freeworkMaxNewPerRun   = 1500
	freeworkDeadlineMargin = 5 * time.Minute
)

// NewFreeWork builds the adapter over the given client.
func NewFreeWork(c HTTPClient) Source {
	return freework{
		get:            c,
		limiter:        rate.NewLimiter(rate.Every(freeworkRequestInterval), 1),
		maxNewPerRun:   freeworkMaxNewPerRun,
		maxPages:       freeworkMaxPages,
		deadlineMargin: freeworkDeadlineMargin,
	}
}

func (freework) Provider() string { return "freework" }

// One national board, no per-tenant id.
func (freework) boardless() {}

// Hundreds of consultancies and agencies post here, each named per posting.
func (freework) aggregator() {}

type freeworkLocation struct {
	Label       string `json:"label"`
	CountryCode string `json:"countryCode"`
}

// freeworkCard is one listing row: what the gate and a body-less Job need.
type freeworkCard struct {
	ID          int64            `json:"id"`
	Slug        string           `json:"slug"`
	Title       string           `json:"title"`
	PublishedAt string           `json:"publishedAt"`
	RemoteMode  *string          `json:"remoteMode"`
	Location    freeworkLocation `json:"location"`
	Company     struct {
		Name string `json:"name"`
	} `json:"company"`
}

// freeworkDetail is the part of the posting resource the adapter reads.
type freeworkDetail struct {
	freeworkCard
	Description      string   `json:"description"`
	CandidateProfile string   `json:"candidateProfile"`
	ExperienceLevel  *string  `json:"experienceLevel"`
	MinAnnualSalary  *int     `json:"minAnnualSalary"`
	MaxAnnualSalary  *int     `json:"maxAnnualSalary"`
	MinDailySalary   *int     `json:"minDailySalary"`
	MaxDailySalary   *int     `json:"maxDailySalary"`
	Currency         *string  `json:"currency"`
	Contracts        []string `json:"contracts"`
	Status           string   `json:"status"`
	Job              *struct {
		Slug string `json:"slug"`
	} `json:"job"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s freework) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s freework) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the whole listing, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest first. A posting the pipeline will discard anyway is judged on
// its listing row and yielded body-less without a request. A posting whose body cannot be read is
// not yielded; a later run tries again.
func (s freework) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, freeworkClean(c.Company.Name))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []freeworkCard
	deferred := 0
	for _, c := range cards {
		base := freeworkBaseJob(c)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, c)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, freeworkDetailWorkers, func(c freeworkCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("freework: posting %d failed; deferring: %v", c.ID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no posting at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("freework: requested %d postings and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("freework: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("freework: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s freework) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the listing page by page and returns every posting with a title and an employer,
// newest first. The client asks for application/json, and in that format the API answers a bare
// list with no total and no "next" link, and answers any page past the end with the LAST page
// again. So the walk ends at a short page, or at a page that names no posting it had not already
// read. The API refuses every ordering parameter, and its default order is the publication
// date, which a republished posting moves to the top mid-walk: a walk sees ~0.1% of the board
// twice (deduplicated here) and, in exchange, misses as many, which is far inside the unseen
// sweep's 48 h. Any page failing fails the crawl — a partial listing would let the
// company-scoped sweep close the postings of the employers it did see on the pages it did not.
func (s freework) list(ctx context.Context) ([]freeworkCard, error) {
	listed := make(map[int64]bool) // every id a page named, for the end-of-list test
	kept := make(map[int64]bool)   // the ids yielded, each once
	var out []freeworkCard
	for page := 1; page <= s.maxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var cards []freeworkCard
		if err := s.get.GetJSON(ctx, fmt.Sprintf(freeworkListURL, page, freeworkPageSize), &cards); err != nil {
			return nil, fmt.Errorf("freework: listing page %d: %w", page, err)
		}
		if page == 1 && len(cards) == 0 {
			return nil, fmt.Errorf("freework: listing carries no posting")
		}
		fresh := 0
		for _, c := range cards {
			if c.ID <= 0 {
				continue
			}
			if !listed[c.ID] {
				listed[c.ID] = true
				fresh++
			}
			if kept[c.ID] || strings.TrimSpace(c.Slug) == "" || freeworkClean(c.Title) == "" || freeworkClean(c.Company.Name) == "" {
				continue
			}
			kept[c.ID] = true
			out = append(out, c)
		}
		if len(cards) < freeworkPageSize || fresh == 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("freework: listing still not exhausted after %d pages", s.maxPages)
}

func freeworkClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// freeworkPostingURL is the posting's public page. The page is addressed by the posting slug
// under its job family's slug; the listing does not carry the family, and the site redirects any
// family segment to the right one, so a listing-only Job uses a placeholder the detail replaces.
func freeworkPostingURL(family, slug string) string {
	if family == "" {
		family = "emploi"
	}
	return freeworkSite + family + "/" + slug
}

// freeworkBaseJob is the body-less Job a listing row alone supports: the identity, and the facts
// the gate judges on.
func freeworkBaseJob(c freeworkCard) Job {
	j := Job{
		ExternalID: strconv.FormatInt(c.ID, 10),
		URL:        freeworkPostingURL("", c.Slug),
		Title:      freeworkClean(c.Title),
		Company:    freeworkClean(c.Company.Name),
		Location:   freeworkClean(c.Location.Label),
	}
	if code := strings.ToLower(strings.TrimSpace(c.Location.CountryCode)); len(code) == 2 {
		j.Countries = []string{code}
	}
	if c.RemoteMode != nil {
		j.WorkMode = freeworkWorkMode(*c.RemoteMode)
		j.Remote = j.WorkMode == "remote"
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(c.PublishedAt)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	return j
}

// detail reads one posting. ok=false with a nil error is the site's answer that the posting is
// gone (404/410, or a status other than "published") or carries no usable body; an error is the
// absence of an answer.
func (s freework) detail(ctx context.Context, c freeworkCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	var d freeworkDetail
	if err := s.get.GetJSON(ctx, fmt.Sprintf(freeworkDetailURL, c.Slug), &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if !strings.EqualFold(strings.TrimSpace(d.Status), "published") {
		return Job{}, false, nil
	}
	job, ok := freeworkJob(d, c)
	return job, ok, nil
}

// freeworkJob maps a posting resource and its listing row to a Job. ok is false for one with no
// title, employer or body.
func freeworkJob(d freeworkDetail, c freeworkCard) (Job, bool) {
	j := freeworkBaseJob(c)
	if t := freeworkClean(d.Title); t != "" {
		j.Title = t
	}
	if co := freeworkClean(d.Company.Name); co != "" {
		j.Company = co
	}
	body := strings.TrimSpace(d.Description)
	// Some posters paste the profile into the description too; it is appended only when the
	// description does not already carry it.
	if p := strings.TrimSpace(d.CandidateProfile); p != "" && !strings.Contains(body, p) {
		body += "\n<h2>Profil recherché</h2>\n" + p
	}
	j.Description = sanitizeHTML(body)
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	family := ""
	if d.Job != nil {
		family = strings.TrimSpace(d.Job.Slug)
	}
	j.URL = freeworkPostingURL(family, c.Slug)
	if l := freeworkClean(d.Location.Label); l != "" {
		j.Location = l
	}
	if code := strings.ToLower(strings.TrimSpace(d.Location.CountryCode)); len(code) == 2 {
		j.Countries = []string{code}
	}
	if d.RemoteMode != nil {
		j.WorkMode = freeworkWorkMode(*d.RemoteMode)
		j.Remote = j.WorkMode == "remote"
	}
	if d.ExperienceLevel != nil {
		j.Seniority = freeworkSeniority(*d.ExperienceLevel)
	}
	j.EmploymentType = freeworkEmploymentType(d.Contracts)
	if lo, hi, cur, period, ok := freeworkSalary(d); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, cur, period
	}
	return j, true
}

// freeworkWorkMode reads the posting's structured remote mode: "full" is remote, "partial"
// hybrid (two thirds of the board), "none" on-site. A null mode states nothing.
func freeworkWorkMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "full":
		return "remote"
	case "partial":
		return "hybrid"
	case "none":
		return "onsite"
	}
	return ""
}

// freeworkSeniority maps the poster's experience band. "expert" is the band past senior, not a
// management level, so it reads as senior; "intermediate" is our middle.
func freeworkSeniority(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "junior":
		return "junior"
	case "intermediate":
		return "middle"
	case "senior", "expert":
		return "senior"
	}
	return ""
}

// freeworkEmploymentType maps the posting's contracts. A posting may offer several (a mission
// "contractor" or "fixed-term"); only a single unambiguous arrangement is mapped.
func freeworkEmploymentType(contracts []string) string {
	kinds := map[string]bool{}
	for _, c := range contracts {
		switch strings.ToLower(strings.TrimSpace(c)) {
		case "permanent":
			kinds["full_time"] = true
		case "contractor", "fixed-term":
			kinds["contract"] = true
		case "internship", "apprenticeship":
			kinds["internship"] = true
		}
	}
	if len(kinds) != 1 {
		return ""
	}
	for k := range kinds {
		return k
	}
	return ""
}

// freeworkSalary reads the structured pay: an annual range for employment, a day rate for a
// mission, each in the posting's stated currency. A posting stating both is read as the annual
// one; a missing bound takes the other one's value; a range that runs backwards is not a figure.
func freeworkSalary(d freeworkDetail) (lo, hi int, currency, period string, ok bool) {
	if d.Currency == nil {
		return 0, 0, "", "", false
	}
	currency = strings.ToUpper(strings.TrimSpace(*d.Currency))
	if len(currency) != 3 {
		return 0, 0, "", "", false
	}
	pick := func(min, max *int) (int, int, bool) {
		switch {
		case min != nil && max != nil:
			return *min, *max, *min > 0 && *max >= *min
		case min != nil:
			return *min, *min, *min > 0
		case max != nil:
			return *max, *max, *max > 0
		}
		return 0, 0, false
	}
	if lo, hi, ok := pick(d.MinAnnualSalary, d.MaxAnnualSalary); ok {
		return lo, hi, currency, "year", true
	}
	if lo, hi, ok := pick(d.MinDailySalary, d.MaxDailySalary); ok {
		return lo, hi, currency, "day", true
	}
	return 0, 0, "", "", false
}
