package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// jobsch adapts JobCloud's two national job boards, jobs.ch (German- and English-speaking
// Switzerland) and jobup.ch (Romandie). Both sites serve one posting pool under one UUID space —
// ~17k of jobup's ~31k postings are also on jobs.ch — so they are one provider: ~52k distinct
// postings from every trade, a few percent IT. It is boardless and an aggregator — the employer
// is read per posting. See the "jobs.ch traps" section of AGENTS.md for the measurements behind
// each rule.
//
// robots.txt disallows the web app's API, so the posting set comes from each site's job
// sitemap, which names nothing but each posting's URL. The newest postings also come from the
// date-sorted search pages, whose server-rendered state names title and employer, so the
// pipeline's gate can turn a posting away before its page is bought. Every other fact is read
// from the posting page's server-rendered React Query state, bought only for postings the
// catalogue lacks; the rest are refreshed by identity.
type jobsch struct {
	http    jobschHTTP
	limiter waiter
	// freshPerRun and backlogPerRun bound one run's posting pages; see jobschFreshPerRun.
	freshPerRun    int
	backlogPerRun  int
	searchPages    int
	deadlineMargin time.Duration
	now            func() time.Time
}

// jobschHTTP is the transport the adapter needs: the gzip sitemaps as a raw stream (served as
// application/x-gzip files, not with a Content-Encoding the transport would undo) and the search
// and posting pages as raw markup, whose state is a script assignment rather than DOM.
type jobschHTTP interface {
	StreamGetter
	TextGetter
}

// jobschSite is one of JobCloud's two boards.
type jobschSite struct {
	// sitemap is the site's posting sitemap: every live posting's page in one language. The
	// other files the sitemap index names list search landing pages, not postings.
	sitemap string
	// search is the date-sorted search, newest publication first, by page number.
	search string
	// posting is a posting's page, by id. A posting only jobup.ch carries answers 404 on jobs.ch.
	posting string
	// itCategory is the site's own top-level "IT / telecommunications" category: the two sites
	// number their category trees independently.
	itCategory string
}

var jobschSites = []jobschSite{
	{
		sitemap:    "https://www.jobs.ch/sitemaps/jobs/de/job/sitemap.job-de.xml.gz",
		search:     "https://www.jobs.ch/de/stellenangebote/?term=&sort-by=date&page=%d",
		posting:    "https://www.jobs.ch/de/stellenangebote/detail/%s/",
		itCategory: "106",
	},
	{
		sitemap:    "https://www.jobup.ch/sitemaps/jobup/fr/job/sitemap.job-fr.xml.gz",
		search:     "https://www.jobup.ch/fr/emplois/?term=&sort-by=date&page=%d",
		posting:    "https://www.jobup.ch/fr/emplois/detail/%s/",
		itCategory: "702",
	},
}

const (
	// jobschMaxSitemapBytes caps one sitemap file as read off the wire (~1 MB compressed for
	// jobs.ch's 38k postings); decodeSitemapBody caps it again once inflated.
	jobschMaxSitemapBytes = 16 << 20
	// jobschRequestInterval paces every request of a run, sitemap, search and posting page alike,
	// through one limiter: one a second, a courtesy ceiling rather than a measured limit (~400
	// pages at that pace drew no refusal, from a residential IP and from the crawl host).
	jobschRequestInterval = time.Second
	// jobschDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the pace.
	jobschDetailWorkers = 2
	// jobschSearchPages is how deep each site's date-sorted search is read: 25 pages of 20 are
	// the ~500 newest publications, about half a day of jobs.ch's (a page past 100 redirects to
	// page 1, so the whole catalogue can never come from here).
	jobschSearchPages = 25
	// jobschDeadlineMargin is the time left to the pipeline's writes: a hydrating crawl writes
	// nothing until FetchNewGated returns.
	jobschDeadlineMargin = 5 * time.Minute
)

// jobschFreshPerRun and jobschBacklogPerRun split one run's 1,800 posting pages (~30 minutes at
// one a second, inside the 2,400 s crawl budget) into two lanes.
//
// The fresh lane takes the postings the search pages named and the gate let through, newest
// first: ~1,200 distinct postings are first published a day across both sites.
//
// The backlog lane takes a window of the remaining unseen sitemap postings that moves every
// hour (hydrationLanes). Those name no employer before their page is bought, so the gate can
// only discard them after the fact; a discarded posting is never stored and so stays unseen, and
// a newest-first backlog would stall behind them once they outnumbered the budget. A fresh lane
// that finds fewer postings than its share hands the rest to the backlog.
const (
	jobschFreshPerRun   = 1000
	jobschBacklogPerRun = 800
)

// NewJobsCh builds the adapter over the given client.
func NewJobsCh(c jobschHTTP) Source {
	return jobsch{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(jobschRequestInterval), 1),
		freshPerRun:    jobschFreshPerRun,
		backlogPerRun:  jobschBacklogPerRun,
		searchPages:    jobschSearchPages,
		deadlineMargin: jobschDeadlineMargin,
		now:            time.Now,
	}
}

func (jobsch) Provider() string { return "jobsch" }

// One national pool, no per-tenant id.
func (jobsch) boardless() {}

// Tens of thousands of employers and agencies post here, each named per posting.
func (jobsch) aggregator() {}

// jobschPosting is one posting the crawl knows of: its id, the site whose page is bought for it,
// and — when a search page named it — its card.
type jobschPosting struct {
	id      string
	site    int
	lastmod string
	card    *jobschCard
}

func (p jobschPosting) url() string { return fmt.Sprintf(jobschSites[p.site].posting, p.id) }

// jobschPostingPath matches a posting URL in either site's sitemap; the UUID is the identity.
var jobschPostingPath = regexp.MustCompile(`/detail/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/?$`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s jobsch) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s jobsch) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists every posting both sitemaps and the newest search pages name, refreshes
// the ones already stored, yields body-less those the pipeline will discard anyway, and reads the
// two lanes of the rest (see jobschFreshPerRun). A posting whose page cannot be read is not
// yielded: nothing is stored for it, so a later run simply tries again.
func (s jobsch) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		var companies []string
		for _, p := range postings {
			if p.card != nil {
				companies = append(companies, p.card.company())
			}
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var fresh, backlog []jobschPosting
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url(), SeenRefresh: true})
			continue
		}
		if p.card == nil {
			backlog = append(backlog, p)
			continue
		}
		if p.card.company() == "" {
			continue // an anonymous posting is dropped (jobschDetail), so its page is not bought
		}
		base := p.card.job(p)
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		fresh = append(fresh, p)
	}
	picked, overflow := fresh, []jobschPosting(nil)
	if len(fresh) > s.freshPerRun {
		picked, overflow = fresh[:s.freshPerRun:s.freshPerRun], fresh[s.freshPerRun:]
	}
	rest := append(append([]jobschPosting(nil), overflow...), backlog...)
	picked = append(picked, hydrationLanes(rest, 0, s.backlogPerRun+s.freshPerRun-len(picked), s.now())...)
	deferred := len(fresh) + len(backlog) - len(picked)

	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, jobschDetailWorkers, func(p jobschPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("jobsch: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no posting page at all is broken, not quiet. "Answered" counts a posting
	// the site reported gone; postings left for want of time are not failures.
	if requested := int64(len(picked)) - outOfTime.Load(); requested > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("jobsch: requested %d posting pages and none answered", requested)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("jobsch: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("jobsch: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.freshPerRun+s.backlogPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s jobsch) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list returns every posting the two sitemaps and the newest search pages name, each once: the
// search-named ones first, in the search's order, then the rest by sitemap lastmod, newest
// first. A posting both sites carry is read from jobs.ch, the site listed first. Any request
// failing fails the crawl: a short list would leave the postings it missed un-refreshed, and the
// sweep would close them.
func (s jobsch) list(ctx context.Context) ([]jobschPosting, error) {
	byID := make(map[string]*jobschPosting)
	var order []string
	add := func(p jobschPosting) *jobschPosting {
		if have, ok := byID[p.id]; ok {
			return have
		}
		byID[p.id] = &p
		order = append(order, p.id)
		return byID[p.id]
	}
	for i, site := range jobschSites {
		doc, err := s.sitemap(ctx, site.sitemap)
		if err != nil {
			return nil, fmt.Errorf("jobsch: sitemap %s: %w", site.sitemap, err)
		}
		n := 0
		for _, u := range doc.URLs {
			id := firstSubmatch(jobschPostingPath, strings.TrimSpace(u.Loc))
			if id == "" {
				continue
			}
			n++
			add(jobschPosting{id: id, site: i, lastmod: u.LastMod})
		}
		if n == 0 {
			return nil, fmt.Errorf("jobsch: sitemap %s names no posting", site.sitemap)
		}
	}
	var named []string
	for i, site := range jobschSites {
		cards, err := s.search(ctx, site)
		if err != nil {
			return nil, err
		}
		for _, c := range cards {
			p := add(jobschPosting{id: c.ID, site: i})
			if p.card == nil {
				p.card = &c
				named = append(named, c.ID)
			}
		}
	}
	var rest []string
	for _, id := range order {
		if byID[id].card == nil {
			rest = append(rest, id)
		}
	}
	// lastmod is an RFC 3339 timestamp; its text order is its time order within one offset,
	// and a posting's exact place in the backlog matters little.
	sort.SliceStable(rest, func(i, j int) bool { return byID[rest[i]].lastmod > byID[rest[j]].lastmod })
	out := make([]jobschPosting, 0, len(order))
	for _, id := range append(named, rest...) {
		out = append(out, *byID[id])
	}
	return out, nil
}

// sitemap fetches and decodes one gzip sitemap file. decodeSitemapBody inflates it to EOF, so a
// truncated file fails gzip's checksum instead of reading as a short list.
func (s jobsch) sitemap(ctx context.Context, url string) (sitemapDoc, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return sitemapDoc{}, err
	}
	var doc sitemapDoc
	err := s.http.GetStream(ctx, url, "application/x-gzip", func(r io.Reader) error {
		body, err := io.ReadAll(io.LimitReader(r, jobschMaxSitemapBytes+1))
		if err != nil {
			return err
		}
		if len(body) > jobschMaxSitemapBytes {
			return fmt.Errorf("sitemap exceeds %d bytes", jobschMaxSitemapBytes)
		}
		doc, err = decodeSitemapBody(body)
		return err
	})
	return doc, err
}

// jobschCard is one search result as the page's state holds it.
type jobschCard struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	Place              string           `json:"place"`
	Locations          []jobschLocation `json:"locations"`
	InitialPublication string           `json:"initialPublicationDate"`
}

func (c jobschCard) company() string { return jobschClean(c.Company.Name) }

// job is the body-less Job a card supports: enough for the gate, and for a posting the gate turns
// away.
func (c jobschCard) job(p jobschPosting) Job {
	return Job{
		ExternalID:        p.id,
		URL:               p.url(),
		Title:             jobschClean(c.Title),
		Company:           c.company(),
		Location:          jobschLocationLabel(c.Locations, c.Place),
		PostedAt:          NotFuture(parseRFC3339(c.InitialPublication)),
		Countries:         jobschCountries(c.Locations),
		FallbackCountries: []string{"ch"},
	}
}

// search reads the site's newest publications, page by page, until searchPages or a page with no
// result. A page past the site's last one redirects to the first page, which is why the walk
// also stops at a page that names nothing new.
func (s jobsch) search(ctx context.Context, site jobschSite) ([]jobschCard, error) {
	var out []jobschCard
	read := make(map[string]bool)
	for page := 1; page <= s.searchPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		url := fmt.Sprintf(site.search, page)
		body, err := s.http.GetText(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("jobsch: search %s: %w", url, err)
		}
		var state struct {
			Vacancy struct {
				Results struct {
					Main struct {
						Results []jobschCard `json:"results"`
					} `json:"main"`
				} `json:"results"`
			} `json:"vacancy"`
		}
		if err := jobschState(body, "__INIT__", &state); err != nil {
			return nil, fmt.Errorf("jobsch: search %s: %w", url, err)
		}
		fresh := 0
		for _, c := range state.Vacancy.Results.Main.Results {
			if c.ID == "" || read[c.ID] {
				continue
			}
			read[c.ID] = true
			fresh++
			out = append(out, c)
		}
		if fresh == 0 {
			break
		}
	}
	return out, nil
}

// detail fetches one posting page and maps it. A posting the site no longer carries answers 404
// and is not yielded, like any other unreadable page; the error separates a failed request from
// the site's own answer that the posting is gone.
func (s jobsch) detail(ctx context.Context, p jobschPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	body, err := s.http.GetText(ctx, p.url())
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var state struct {
		Queries []struct {
			Key   []json.RawMessage `json:"queryKey"`
			State struct {
				Data json.RawMessage `json:"data"`
			} `json:"state"`
		} `json:"queries"`
	}
	if err := jobschState(body, "__REACT_QUERY_STATE__", &state); err != nil {
		return Job{}, false, err
	}
	for _, q := range state.Queries {
		var key string
		if len(q.Key) == 0 || json.Unmarshal(q.Key[0], &key) != nil || key != "vacancy-detail" {
			continue
		}
		var v jobschVacancy
		if err := json.Unmarshal(q.State.Data, &v); err != nil {
			return Job{}, false, err
		}
		if v.ID != p.id {
			return Job{}, false, fmt.Errorf("page state is posting %q", v.ID)
		}
		job, ok := jobschDetail(v, p)
		return job, ok, nil
	}
	return Job{}, false, errors.New("page state carries no vacancy")
}

// jobschVacancy is the posting as the page's state holds it.
type jobschVacancy struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Active  bool   `json:"isActive"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	Text               string           `json:"text"`
	Place              string           `json:"place"`
	Locations          []jobschLocation `json:"locations"`
	InitialPublication string           `json:"initialPublicationDate"`
	EmploymentTypes    []string         `json:"employmentTypeIds"`
	Grades             []int            `json:"employmentGrades"`
	Categories         []struct {
		Top json.Number `json:"lvl_0"`
	} `json:"categories"`
	Benefits []string      `json:"benefits"`
	Salary   *jobschSalary `json:"salary"`
}

type jobschLocation struct {
	City        string `json:"city"`
	CountryCode string `json:"countryCode"`
}

// jobschSalary is the employer's own salary: a unit, a currency and a range whose top may be
// missing. JobCloud's salary ESTIMATE is a separate query the adapter never reads.
type jobschSalary struct {
	Unit     string `json:"unit"`
	Currency string `json:"currency"`
	Range    struct {
		Min *float64 `json:"minValue"`
		Max *float64 `json:"maxValue"`
	} `json:"range"`
}

// jobschDetail maps a posting's state to a Job. ok is false for an inactive posting, and for one
// without a title, employer or body: storing it body-less would make it seen, and a seen posting
// is never read again. An anonymous posting (a recruiter's "for a client", ~1%) names no
// employer, and filing it would build one bogus company out of many.
func jobschDetail(v jobschVacancy, p jobschPosting) (Job, bool) {
	title := jobschClean(v.Title)
	company := jobschClean(v.Company.Name)
	body := sanitizeHTML(v.Text)
	if !v.Active || title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	site := jobschSites[p.site]
	job := Job{
		ExternalID:        p.id,
		URL:               p.url(),
		Title:             title,
		Company:           company,
		Location:          jobschLocationLabel(v.Locations, v.Place),
		Description:       body,
		PostedAt:          NotFuture(parseRFC3339(v.InitialPublication)),
		EmploymentType:    jobschEmploymentType(v.EmploymentTypes, v.Grades),
		Countries:         jobschCountries(v.Locations),
		FallbackCountries: []string{"ch"},
	}
	for _, c := range v.Categories {
		if c.Top.String() == site.itCategory {
			job.IsTechHint = true
		}
	}
	for _, b := range v.Benefits {
		// The employer ticked "working from home" among the benefits; the posting is still tied
		// to its place, so it is hybrid, never remote.
		if b == "working-from-home" {
			job.WorkMode = "hybrid"
		}
	}
	if lo, hi, period, ok := v.Salary.bounds(); ok {
		job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, strings.ToUpper(v.Salary.Currency), period
	}
	return job, true
}

// jobschMinYearlySalary is the smallest figure read as a yearly salary. An apprenticeship stated
// as YEAR 1,020-1,550 CHF was a monthly wage under the wrong unit; no Swiss year is that small.
const jobschMinYearlySalary = 10000

// bounds returns the salary as whole figures with its period, or ok=false when there is none,
// when the unit is not one freehire knows, or when a yearly figure is too small to be a year.
func (s *jobschSalary) bounds() (lo, hi *int, period string, ok bool) {
	if s == nil || s.Currency == "" || s.Range.Min == nil || *s.Range.Min <= 0 {
		return nil, nil, "", false
	}
	switch s.Unit {
	case "YEAR":
		period = "year"
	case "MONTH":
		period = "month"
	case "DAY":
		period = "day"
	case "HOUR":
		period = "hour"
	default:
		return nil, nil, "", false
	}
	minimum := int(*s.Range.Min)
	maximum := minimum
	if s.Range.Max != nil && int(*s.Range.Max) >= minimum {
		maximum = int(*s.Range.Max)
	}
	if period == "year" && minimum < jobschMinYearlySalary {
		return nil, nil, "", false
	}
	return &minimum, &maximum, period, true
}

// jobschEmploymentType maps the site's employment type when it states exactly one. A permanent
// position ("Festanstellung") says nothing of the hours, so the workload grades decide: 100% is
// full time, a range topping out below it part time, and "80-100%" neither. An apprenticeship
// ("Lehrstelle") is a years-long vocational training, which no freehire type names.
func jobschEmploymentType(ids []string, grades []int) string {
	if len(ids) != 1 {
		return ""
	}
	switch ids[0] {
	case "1", "2": // Temporär, Freelance
		return "contract"
	case "3": // Praktikum
		return "internship"
	case "4": // Nebenerwerb
		return "part_time"
	case "5": // Festanstellung
		if len(grades) == 2 && grades[0] == 100 && grades[1] == 100 {
			return "full_time"
		}
		if len(grades) == 2 && grades[1] > 0 && grades[1] < 100 {
			return "part_time"
		}
	}
	return ""
}

// jobschLocationLabel names the posting's places by city. The canton code is left out: "BE" and
// "AG" read as countries to the location dictionary, and the structured country already says
// where the posting is.
func jobschLocationLabel(locs []jobschLocation, place string) string {
	if l := distinctJoin(locs, "; ", func(l jobschLocation) string { return jobschClean(l.City) }); l != "" {
		return l
	}
	return jobschClean(place)
}

func jobschCountries(locs []jobschLocation) []string {
	codes := make([]string, 0, len(locs))
	for _, l := range locs {
		codes = append(codes, l.CountryCode)
	}
	return countriesFromCodes(codes)
}

func jobschClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// jobschState decodes one of the page's state assignments ("      __INIT__ = {...};") into v. The
// value is a JavaScript object literal: JSON except that a missing field is written as a bare
// undefined, which is rewritten to null before decoding. The decoder stops at the end of the
// value, so what follows the assignment is never read.
func jobschState(page, name string, v any) error {
	marker := name + " = "
	i := strings.Index(page, marker)
	if i < 0 {
		return fmt.Errorf("page carries no %s", name)
	}
	if err := json.NewDecoder(strings.NewReader(jsUndefinedToNull(page[i+len(marker):]))).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// jsUndefinedToNull rewrites every bare undefined token outside a string literal to null, so a
// JavaScript object literal decodes as JSON. Inside strings nothing changes, escapes included.
func jsUndefinedToNull(s string) string {
	const token = "undefined"
	if !strings.Contains(s, token) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inString, escaped := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inString:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == 'u' && strings.HasPrefix(s[i:], token) && !jsIdentByte(s, i-1) && !jsIdentByte(s, i+len(token)):
			b.WriteString("null")
			i += len(token) - 1
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// jsIdentByte reports whether s[i] exists and could continue an identifier, so "undefinedX" or
// "xundefined" is never mistaken for the token.
func jsIdentByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c == '_' || c == '$' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
