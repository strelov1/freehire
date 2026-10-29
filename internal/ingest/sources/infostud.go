package sources

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// infostud adapts Poslovi Infostud (poslovi.infostud.com), Serbia's largest general job board:
// every trade, ~3% technical, thousands of employers each named per posting. It is boardless and
// an aggregator. See the "Infostud traps" section of AGENTS.md for the measurements behind each
// rule.
//
// The listing is the site's own server-rendered search, 30 postings a page, whose __NEXT_DATA__
// carries every structured fact but the body. The body and the structured salary come from the
// posting's own page, bought only for postings the catalogue lacks, within a per-run budget; the
// rest are refreshed by identity. The GraphQL API both pages are built from is disallowed by its
// host's robots.txt, so it is not used.
type infostud struct {
	http           TextGetter
	limiter        waiter
	maxNewPerRun   int
	maxPages       int
	deadlineMargin time.Duration
}

const (
	infostudBaseURL = "https://poslovi.infostud.com"
	// infostudListURL is the unfiltered search. Its default order puts paid placements first, so
	// it is not newest first; a page past the last answers page 1 again rather than nothing.
	infostudListURL   = infostudBaseURL + "/oglasi-za-posao?page=%d"
	infostudPostURL   = infostudBaseURL + "/posao/%s/%s/%d"
	infostudPageSize  = 30
	infostudDetailers = 2

	// infostudRequestInterval paces every request of a run at one a second — a courtesy
	// ceiling, not a measured limit.
	infostudRequestInterval = time.Second
	// infostudMaxPages is a safety ceiling: the search held 3,171 postings (106 pages) on
	// 2026-10-28. Reaching it fails the crawl rather than ending it.
	infostudMaxPages = 500
	// infostudMaxNewPerRun bounds the posting pages of one run: ~106 listing pages and 1,000
	// posting pages at one a second, plus the writes, sit inside the scheduler's default 2,400 s
	// crawl budget. A cold catalogue is taken in slices over a few runs.
	infostudMaxNewPerRun   = 1000
	infostudDeadlineMargin = 5 * time.Minute
)

// NewInfostud builds the adapter over the given client.
func NewInfostud(c TextGetter) Source {
	return infostud{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(infostudRequestInterval), 1),
		maxNewPerRun:   infostudMaxNewPerRun,
		maxPages:       infostudMaxPages,
		deadlineMargin: infostudDeadlineMargin,
	}
}

func (infostud) Provider() string { return "infostud" }

// One national search, no per-tenant id.
func (infostud) boardless() {}

// Thousands of employers post here, each named per posting.
func (infostud) aggregator() {}

// infostudCard is one search result: what the gate and the job need before the body is bought.
type infostudCard struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	Company         string `json:"companyName"`
	Location        string `json:"location"`
	OnlineViewDate  string `json:"onlineViewDate"`
	WorkFromHome    bool   `json:"workFromHome"`
	HybridWork      bool   `json:"hybridWork"`
	URL             string `json:"url"`
	PrimaryCategory *struct {
		ID int `json:"id"`
	} `json:"primaryCategory"`
}

type infostudListData struct {
	Props struct {
		PageProps struct {
			Results *struct {
				Total int `json:"totalPrimaryItems"`
				Page  int `json:"page"`
				Jobs  struct {
					Primary []infostudCard `json:"primary"`
				} `json:"jobs"`
			} `json:"initialSearchResults"`
		} `json:"pageProps"`
	} `json:"props"`
}

// infostudPosting is the part of a posting page's job the adapter reads.
type infostudPosting struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Company    string `json:"companyName"`
	Display    string `json:"companyDisplayName"`
	Active     bool   `json:"active"`
	Status     string `json:"status"`
	TextAd     string `json:"textAd"`
	DatePosted string `json:"datePosted"`
	Location   string `json:"location"`
	Salary     *struct {
		From     *string `json:"from"`
		To       *string `json:"to"`
		Period   string  `json:"additional"`
		Type     string  `json:"type"`
		Currency string  `json:"currency"`
	} `json:"unformattedSalary"`
	SalaryLabel     *string `json:"salary"`
	PrimaryCategory *struct {
		ID int `json:"id"`
	} `json:"primaryCategory"`
	WorkingHours *struct {
		Name string `json:"nameSr"`
	} `json:"workingHours"`
}

type infostudPostData struct {
	Props struct {
		PageProps struct {
			Job *infostudPosting `json:"job"`
		} `json:"pageProps"`
	} `json:"props"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s infostud) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s infostud) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the whole search, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, freshest first. A posting the pipeline will discard anyway is judged on
// its search result and yielded body-less without a request. A posting whose page cannot be read
// is not yielded; a later run tries again.
func (s infostud) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, infostudClean(c.Company))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []infostudCard
	for _, c := range cards {
		base := infostudBaseJob(c)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		unseen = append(unseen, c)
	}
	// The search is not ordered by date, so the budget goes to the freshest postings first: an
	// older one is closer to its expiry and the more likely to be covered elsewhere already.
	sortInfostudFreshestFirst(unseen)
	deferred := 0
	if len(unseen) > s.maxNewPerRun {
		deferred = len(unseen) - s.maxNewPerRun
		unseen = unseen[:s.maxNewPerRun]
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, infostudDetailers, func(c infostudCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("infostud: posting %d failed; deferring: %v", c.ID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no posting at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("infostud: requested %d posting pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("infostud: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("infostud: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s infostud) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks every page the search's own total implies. Any page failing fails the crawl — a
// partial listing would let the company-scoped sweep close the postings of the employers it saw
// on the pages it did not read. A page that answers as another page (the site serves page 1 past
// the last one) ends the walk.
func (s infostud) list(ctx context.Context) ([]infostudCard, error) {
	seen := make(map[int64]bool)
	var out []infostudCard
	pages := 1
	for page := 1; page <= pages; page++ {
		if page > s.maxPages {
			return nil, fmt.Errorf("infostud: listing still not exhausted after %d pages", s.maxPages)
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		body, err := s.http.GetText(ctx, fmt.Sprintf(infostudListURL, page))
		if err != nil {
			return nil, fmt.Errorf("infostud: listing page %d: %w", page, err)
		}
		var d infostudListData
		if err := infostudNextData(body, &d); err != nil {
			return nil, fmt.Errorf("infostud: listing page %d: %w", page, err)
		}
		r := d.Props.PageProps.Results
		if r == nil {
			return nil, fmt.Errorf("infostud: listing page %d carries no search results", page)
		}
		if page == 1 {
			if r.Total <= 0 || len(r.Jobs.Primary) == 0 {
				return nil, fmt.Errorf("infostud: listing carries no posting")
			}
			pages = (r.Total + infostudPageSize - 1) / infostudPageSize
		}
		if r.Page != page {
			// Postings closing during the walk shrink the search, so its end can come a page
			// before the total read on page 1 implied.
			break
		}
		for _, c := range r.Jobs.Primary {
			if c.ID <= 0 || seen[c.ID] || infostudClean(c.Title) == "" || infostudClean(c.Company) == "" {
				continue
			}
			seen[c.ID] = true
			out = append(out, c)
		}
	}
	return out, nil
}

// infostudNextData decodes a page's __NEXT_DATA__ script into v.
func infostudNextData(page string, v any) error {
	raw, ok := bracketSlice(page, `id="__NEXT_DATA__"`, '{', '}')
	if !ok {
		return errors.New("page carries no __NEXT_DATA__")
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("decode __NEXT_DATA__: %w", err)
	}
	return nil
}

func infostudClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// infostudPostingURL is the posting's page. Only the trailing id is read — any two slugs answer
// the same page — so the stored URL is the one the search names when it names one.
func infostudPostingURL(c infostudCard) string {
	if strings.HasPrefix(c.URL, infostudBaseURL+"/posao/") {
		return c.URL
	}
	return fmt.Sprintf(infostudPostURL, "oglas", "poslodavac", c.ID)
}

// infostudBaseJob is the body-less Job a search result alone supports: the identity, and the
// facts the gate judges on.
func infostudBaseJob(c infostudCard) Job {
	mode := infostudWorkMode(c.WorkFromHome, c.HybridWork)
	return Job{
		ExternalID:        strconv.FormatInt(c.ID, 10),
		URL:               infostudPostingURL(c),
		Title:             infostudClean(c.Title),
		Company:           infostudClean(c.Company),
		Location:          infostudPlace(c.Location),
		WorkMode:          mode,
		Remote:            mode == "remote",
		IsTechHint:        c.PrimaryCategory != nil && infostudTechCategory(c.PrimaryCategory.ID),
		FallbackCountries: []string{"rs"},
	}
}

// infostudWorkMode reads the search's two structured flags. Neither set is silence, not on-site:
// "Iz firme" is the form's default for most postings.
func infostudWorkMode(fromHome, hybrid bool) string {
	switch {
	case fromHome:
		return "remote"
	case hybrid:
		return "hybrid"
	}
	return ""
}

// infostudPlace strips the work-arrangement suffixes the site appends to a place ("Beograd |
// Hibrid", "Beograd | Terenski rad") and the bare "Rad od kuće" it shows for a remote posting;
// the arrangement itself is read from the structured flags.
func infostudPlace(s string) string {
	var parts []string
	for _, p := range strings.Split(s, "|") {
		p = infostudClean(p)
		switch strings.ToLower(p) {
		case "", "hibrid", "terenski rad", "rad od kuće", "rad od kuce":
			continue
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

// infostudTechCategory reports the site's own "IT" category (5). Telecommunications (29) is not
// included: it holds field technicians and call-centre sales as much as engineers.
func infostudTechCategory(id int) bool { return id == 5 }

// infostudOnlineDate reads the search's "26.10.2026" publication date.
func infostudOnlineDate(s string) (time.Time, bool) {
	t, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	return t, err == nil
}

func sortInfostudFreshestFirst(cards []infostudCard) {
	key := func(c infostudCard) int64 {
		if t, ok := infostudOnlineDate(c.OnlineViewDate); ok {
			return t.Unix()
		}
		return 0
	}
	// Stable, so the search's own order is kept inside a day.
	slices.SortStableFunc(cards, func(a, b infostudCard) int { return cmp.Compare(key(b), key(a)) })
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404, or a status other than "live") or carries no body; an error is the absence of an
// answer.
func (s infostud) detail(ctx context.Context, c infostudCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	body, err := s.http.GetText(ctx, infostudPostingURL(c))
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var d infostudPostData
	if err := infostudNextData(body, &d); err != nil {
		return Job{}, false, err
	}
	p := d.Props.PageProps.Job
	if p == nil {
		return Job{}, false, errors.New("posting page carries no job")
	}
	if !p.Active || !strings.EqualFold(strings.TrimSpace(p.Status), "live") {
		return Job{}, false, nil
	}
	job, ok := infostudJob(*p, c)
	return job, ok, nil
}

// infostudBelgrade is the zone the site's dates are written in.
var infostudBelgrade = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Belgrade"); err == nil {
		return loc
	}
	return time.FixedZone("CET", 60*60)
}()

// infostudJob maps a posting page and its search result to a Job. ok is false for one with no
// title, employer or body — an ad published only as an image carries no text.
func infostudJob(p infostudPosting, c infostudCard) (Job, bool) {
	j := infostudBaseJob(c)
	if t := infostudClean(p.Title); t != "" {
		j.Title = t
	}
	if co := infostudClean(p.Company); co != "" {
		j.Company = co
	}
	body := strings.TrimSpace(p.TextAd)
	if j.Title == "" || j.Company == "" || body == "" {
		return Job{}, false
	}
	if lo, hi, cur, period, ok := infostudSalary(p); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, hi, cur, period
		// The figure is net or gross as the poster chose, and the Job has no field for it; the
		// site's own label ("neto 900 - 1.400 EUR (mesečno)") keeps it in view.
		if p.SalaryLabel != nil {
			if label := infostudClean(*p.SalaryLabel); label != "" {
				body = "<p>Plata: " + label + "</p>" + body
			}
		}
	}
	j.Description = sanitizeHTML(body)
	if strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	if place := infostudPlace(p.Location); place != "" {
		j.Location = place
	}
	if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(p.DatePosted), infostudBelgrade); err == nil {
		j.PostedAt = NotFuture(&t)
	} else if t, ok := infostudOnlineDate(c.OnlineViewDate); ok {
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, infostudBelgrade)
		j.PostedAt = NotFuture(&t)
	}
	if p.PrimaryCategory != nil {
		j.IsTechHint = infostudTechCategory(p.PrimaryCategory.ID)
	}
	if p.WorkingHours != nil && strings.EqualFold(infostudClean(p.WorkingHours.Name), "nepuno radno vreme") {
		j.EmploymentType = "part_time"
	}
	return j, true
}

// infostudSalary reads the posting's structured salary. The figures are in hundredths of the
// currency ("4000" is 40.00 USD an hour), and a posting states its period and whether the figure
// is net or gross; one missing the lower bound, the period, the currency or that statement yields
// nothing. A posting stating one figure ("neto 200.000 RSD") has no upper bound, so hi is nil.
func infostudSalary(p infostudPosting) (lo int, hi *int, currency, period string, ok bool) {
	sal := p.Salary
	if sal == nil || sal.From == nil {
		return 0, nil, "", "", false
	}
	switch strings.ToLower(strings.TrimSpace(sal.Period)) {
	case "monthly":
		period = "month"
	case "hourly":
		period = "hour"
	case "yearly", "annually":
		period = "year"
	default:
		return 0, nil, "", "", false
	}
	currency = strings.ToUpper(strings.TrimSpace(sal.Currency))
	switch currency {
	case "RSD", "EUR", "USD", "CHF", "GBP":
	default:
		return 0, nil, "", "", false
	}
	if t := strings.ToLower(strings.TrimSpace(sal.Type)); t != "net" && t != "gross" {
		return 0, nil, "", "", false
	}
	from, err := strconv.Atoi(strings.TrimSpace(*sal.From))
	if err != nil || from <= 0 {
		return 0, nil, "", "", false
	}
	lo = from / 100
	if sal.To != nil && strings.TrimSpace(*sal.To) != "" {
		to, err := strconv.Atoi(strings.TrimSpace(*sal.To))
		if err != nil || to < from {
			return 0, nil, "", "", false
		}
		v := to / 100
		hi = &v
	}
	return lo, hi, currency, period, true
}
