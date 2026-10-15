package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// builtin adapts Built In (builtin.com), a US-centred tech job board that also lists employers'
// postings worldwide. The provider is a boardless aggregator: every Built In market host
// (builtinnyc.com, builtinsf.com, ...) serves the same platform and the same posting ids, and
// builtin.com with allLocations=true lists all of them.
//
// The whole catalogue cannot be walked. The listing stops at page 1000 (~12-25 cards a page)
// whatever the filter, the unfiltered tech categories alone outgrow that within a week, and the
// edge challenges an IP that exceeds ~40 requests a minute. So a run reads a WINDOW: each tech
// category's postings updated in the last 24 hours, to that listing's natural end. A posting is
// therefore refreshed on the runs inside its first day and again whenever Built In reposts it,
// and the sweep waits builtinSweepGrace — the 30 days Built In itself states as a posting's
// validThrough — before calling an unseen posting gone.
//
// Most postings belong to employers freehire already crawls first-party (≥70% of a 168-posting
// sample on 2026-10-15), so the adapter is CoverageGated: their bodies are never bought. See the
// Built In traps in AGENTS.md.
type builtin struct {
	http HTMLGetter
	// maxNewPerRun bounds how many posting pages one run hydrates; the rest wait for a later
	// run, newest category order first.
	maxNewPerRun int
}

const (
	builtinBase = "https://builtin.com"
	// builtinMaxPages is the listing's own ceiling: page 1001 of any filter answers "No job
	// results", so a window that reached page 1000 was truncated, not exhausted.
	builtinMaxPages = 1000
	// builtinWindowDays is the listing's narrowest "posted/updated within" option.
	builtinWindowDays = 1
	// builtinSweepGrace is Built In's own posting lifetime: every sampled JobPosting states a
	// validThrough exactly 30 days after its datePosted.
	builtinSweepGrace = 30 * 24 * time.Hour

	builtinDefaultMaxNewPerRun = 500
	// The pacer serializes requests, so more workers would only queue on it.
	builtinDetailWorkers = 2
)

// Built In's Cloudflare edge answers a managed challenge (HTTP 429, cf-mitigated: challenge)
// once an IP exceeds its budget: measured 2026-10-15 from a residential IP, one request every
// 0.5s was refused from the 44th request, ~0.85 req/s from the ~140th, while one every 1.5s
// (200 requests) and one every 3s (100 requests) were never refused. The pace sits below the
// highest rate that held, because the shared client retries a refused request beneath this
// limiter. At this pace one live run listed 4,001 postings in 10 minutes; at most
// builtinDefaultMaxNewPerRun posting pages add ~17 more.
const (
	builtinRequestInterval = 2 * time.Second
	builtinRequestBurst    = 1
)

// builtinCategories are the listing's top-level job families (its own category pages,
// /jobs/<family>), in the order hydration spends its budget on them: the technical families
// first, then the rest, since the catalogue is broader than IT. The first version walked six
// technical sub-categories (engineering ~326 pages a day, data-analytics ~60 on 2026-10-15);
// every family answered its window on 2026-10-22. Each family's day must stay inside the
// listing's 1000-page ceiling.
var builtinCategories = []string{
	"dev-engineering",
	"data-analytics",
	"cybersecurity-it",
	"product",
	"design-ux",
	"project-management",
	"operations",
	"customer-success",
	"sales",
	"marketing",
	"finance",
	"hr",
	"legal",
	"content",
}

func NewBuiltIn(c HTMLGetter) Source {
	return builtin{http: c, maxNewPerRun: builtinDefaultMaxNewPerRun}
}

func (builtin) Provider() string          { return "builtin" }
func (builtin) boardless()                {}
func (builtin) aggregator()               {}
func (builtin) sweepGrace() time.Duration { return builtinSweepGrace }

// builtinCard is what a listing card states about a posting: enough for identity, for the
// coverage gate (employer) and for the catalogue filter (title).
type builtinCard struct {
	ID       string
	URL      string
	Title    string
	Company  string
	WorkMode string
}

func (c builtinCard) job() Job {
	return Job{
		ExternalID: c.ID,
		URL:        c.URL,
		Title:      c.Title,
		Company:    c.Company,
		WorkMode:   c.WorkMode,
		Remote:     c.WorkMode == "remote",
	}
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: every listed
// posting, body-less.
func (s builtin) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(cards))
	for _, c := range cards {
		jobs = append(jobs, c.job())
	}
	return jobs, nil
}

func (s builtin) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, cards, seen, nil, func(Job) bool { return false })
}

// FetchNewGated is FetchNew told what the pipeline is going to discard. On Built In that is
// most of the listing: the employers mostly run their own ATS, which freehire already crawls,
// and a discarded posting is never stored, so without the gate every run would buy its body
// again.
func (s builtin) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cards))
	for _, c := range cards {
		names = append(names, c.Company)
	}
	return s.hydrate(ctx, cards, seen, gate.Covered(names), gate.Rejected)
}

// list walks every category's window and returns its cards, each posting once. A category's
// walk follows the ordinary rule: its first page failing fails the run, a later page failing
// ends that category with what it gathered.
func (s builtin) list(ctx context.Context) ([]builtinCard, error) {
	var out []builtinCard
	listed := map[string]bool{}
	for _, category := range builtinCategories {
		byURL := map[string]builtinCard{}
		urls, err := crawlPagedLinks(ctx, s.http, builtinMaxPages, builtinPageURL(category), func(root *xhtml.Node) []string {
			var links []string
			for _, c := range parseBuiltInCards(root) {
				byURL[c.URL] = c
				links = append(links, c.URL)
			}
			return links
		})
		if err != nil {
			return nil, fmt.Errorf("builtin: %s listing: %w", category, err)
		}
		for _, u := range urls {
			c := byURL[u]
			if !listed[c.ID] { // a posting filed under two categories is one posting
				listed[c.ID] = true
				out = append(out, c)
			}
		}
	}
	// Built In lists thousands of postings a day, so a run that found none read a challenge
	// page or a changed markup, never a quiet market.
	if len(out) == 0 {
		return nil, fmt.Errorf("builtin: no category listing carried a job card")
	}
	return out, nil
}

// builtinPageURL is the category's window listing. allLocations=true is load-bearing: without it
// the listing is narrowed to the requesting IP's own region. The page parameter comes LAST, the
// shape the site's own pagination links take.
func builtinPageURL(category string) func(page int) string {
	base := builtinBase + "/jobs/" + category + "?allLocations=true&daysSinceUpdated=" + strconv.Itoa(builtinWindowDays)
	return func(page int) string {
		if page == 1 {
			return base
		}
		return base + "&page=" + strconv.Itoa(page)
	}
}

// builtinWorkModes maps the card's work-arrangement badge. "Remote or Hybrid" states two
// arrangements and is not mapped.
var builtinWorkModes = map[string]string{
	"Remote":    "remote",
	"Hybrid":    "hybrid",
	"In-Office": "onsite",
}

// parseBuiltInCards reads every job card on a listing page: the element marked
// data-id="job-card", whose title link carries the posting's numeric id. A card missing its id,
// title or employer is skipped.
func parseBuiltInCards(root *xhtml.Node) []builtinCard {
	var out []builtinCard
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || Attr(n, "data-id") != "job-card" {
			return true
		}
		var c builtinCard
		walk(n, func(m *xhtml.Node) bool {
			if m.Type != xhtml.ElementNode {
				return true
			}
			switch Attr(m, "data-id") {
			case "company-title":
				c.Company = strings.Join(strings.Fields(textContent(m)), " ")
			case "job-card-title":
				c.ID = strings.TrimSpace(Attr(m, "data-builtin-track-job-id"))
				c.Title = strings.Join(strings.Fields(textContent(m)), " ")
				if href := strings.TrimSpace(Attr(m, "href")); strings.HasPrefix(href, "/job/") {
					c.URL = builtinBase + href
				}
			}
			return true
		})
		c.WorkMode = builtinWorkModes[builtinIconText(n, "fa-house-building")]
		if c.ID != "" && c.URL != "" && c.Title != "" && c.Company != "" {
			out = append(out, c)
		}
		return false
	})
	return out
}

// builtinIconText reads the value a card states beside one of its icons: the card renders each
// attribute as a row holding the icon (in its own wrapper) and a <span> with the value.
func builtinIconText(card *xhtml.Node, icon string) string {
	text := ""
	walk(card, func(n *xhtml.Node) bool {
		if text != "" {
			return false
		}
		if n.Type != xhtml.ElementNode || n.Data != "i" || !HasClass(n, icon) || n.Parent == nil || n.Parent.Parent == nil {
			return true
		}
		for s := n.Parent.Parent.FirstChild; s != nil; s = s.NextSibling {
			if s.Type == xhtml.ElementNode && s.Data == "span" {
				text = strings.Join(strings.Fields(textContent(s)), " ")
				break
			}
		}
		return false
	})
	return text
}

// hydrate buys a posting page only for a posting the pipeline will keep, within the run's
// budget. A covered or rejected posting is still yielded, body-less, so the pipeline makes and
// counts the decision.
//
// A new posting whose page could not be read is DEFERRED rather than stored body-less: a stored
// row is seen and would never be hydrated again once past pipeline.HydrationRetryWindow, while a
// deferred one is simply new on a later run. Once the edge refuses one request the rest of the
// run's hydration is deferred too: the refusal is about the IP, and every further request would
// be refused alike and only prolong it.
func (s builtin) hydrate(ctx context.Context, cards []builtinCard, seen func(string) bool,
	covered map[string]bool, rejected func(Job) bool) ([]Job, error) {
	var out, toHydrate []Job
	deferred := 0
	for _, c := range cards {
		base := c.job()
		switch {
		case seen(c.ID):
			base.SeenRefresh = true
			out = append(out, base)
		case covered[c.Company] || rejected(base):
			out = append(out, base)
		case len(toHydrate) < s.maxNewPerRun:
			toHydrate = append(toHydrate, base)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("builtin: hydration budget %d reached; %d new postings wait for a later run", s.maxNewPerRun, deferred)
	}

	var refused atomic.Bool
	var failed atomic.Int64
	hydrated := fetchDetails(toHydrate, builtinDetailWorkers, func(base Job) (Job, bool) {
		if refused.Load() {
			return Job{}, false
		}
		root, err := s.http.GetHTML(ctx, base.URL)
		if err != nil {
			if isRateLimited(err) && !refused.Swap(true) {
				log.Printf("builtin: posting pages refused (%v); deferring the rest of this run's hydration", err)
			}
			if detailUnreadable(err) { // a 404/410 is the posting gone, which is an answer
				failed.Add(1)
			}
			return Job{}, false
		}
		job, ok := parseBuiltInPosting(root, base)
		if !ok {
			failed.Add(1)
			log.Printf("builtin: %s carries no usable JobPosting; deferring", base.URL)
			return Job{}, false
		}
		return job, true
	})
	if n := failed.Load(); n > 0 {
		log.Printf("builtin: %d/%d posting pages could not be read; deferred to a later run", n, len(toHydrate))
	}
	// A run that needed bodies and read none of them is broken, not quiet.
	if failed.Load() > 0 && len(hydrated) == 0 {
		return nil, fmt.Errorf("builtin: needed %d posting pages and read none of them", len(toHydrate))
	}
	return append(out, hydrated...), nil
}

// builtinPosting is the subset of the posting page's schema.org JobPosting the adapter maps.
type builtinPosting struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	JobLocation        schemaPlaces    `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// parseBuiltInPosting completes the card's Job from the posting page. The employer and title the
// page states win over the card's; the card's work-arrangement badge stays, since the page only
// marks a fully remote posting (jobLocationType TELECOMMUTE).
func parseBuiltInPosting(root *xhtml.Node, base Job) (Job, bool) {
	var p builtinPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	description := sanitizeHTML(p.Description)
	if strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	job := base
	job.Description = description
	if title := strings.Join(strings.Fields(p.Title), " "); title != "" {
		job.Title = title
	}
	if company := strings.TrimSpace(p.HiringOrganization.Name); company != "" {
		job.Company = company
	}
	if job.WorkMode == "" && strings.EqualFold(p.JobLocationType, "TELECOMMUTE") {
		job.WorkMode = "remote"
	}
	job.Remote = job.WorkMode == "remote"
	job.Location, job.Countries = builtinPlaces(p.JobLocation)
	job.PostedAt = NotFuture(parseDate(strings.TrimSpace(p.DatePosted)))
	job.EmploymentType = schemaEmploymentType(builtinEmploymentType(p.EmploymentType))
	builtinSalary(&job, p)
	return job, true
}

// builtinEmploymentType reads employmentType, which schema.org allows as one enum or a list.
func builtinEmploymentType(raw json.RawMessage) string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return many[0]
	}
	return ""
}

// builtinPlaces renders each place as "City, Region, Country" (several joined by "; ") and
// collects their countries, which Built In states as alpha-3 codes ("USA", "AUS").
func builtinPlaces(places schemaPlaces) (string, []string) {
	var parts, countries []string
	seenPart, seenCountry := map[string]bool{}, map[string]bool{}
	for _, pl := range places {
		if part := pl.Address.Location(); part != "" && !seenPart[part] {
			seenPart[part] = true
			parts = append(parts, part)
		}
		if c := location.NormalizeCountry(pl.Address.AddressCountry); c != "" && !seenCountry[c] {
			seenCountry[c] = true
			countries = append(countries, c)
		}
	}
	return strings.Join(parts, "; "), countries
}

var builtinSalaryPeriods = map[string]string{
	"HOUR":  "hour",
	"DAY":   "day",
	"MONTH": "month",
	"YEAR":  "year",
}

// builtinMaxHourlyUSD rejects Built In's own data error: an annual range filed as HOUR (a PwC
// internship states 29250-48000 USD/HOUR, which its card renders as "29K-48K Hourly"). Nobody
// is paid $500 an hour on a job board, and the figure cannot be reinterpreted without guessing
// which period was meant, so such a salary is dropped.
const builtinMaxHourlyUSD = 500

func builtinSalary(job *Job, p builtinPosting) {
	period := builtinSalaryPeriods[strings.ToUpper(strings.TrimSpace(p.BaseSalary.Value.UnitText))]
	currency := strings.ToUpper(strings.TrimSpace(p.BaseSalary.Currency))
	lo, hi := roundSalaryPart(p.BaseSalary.Value.MinValue), roundSalaryPart(p.BaseSalary.Value.MaxValue)
	if period == "" || len(currency) != 3 || (lo == nil && hi == nil) {
		return
	}
	if lo != nil && hi != nil && *lo > *hi {
		return
	}
	if period == "hour" && currency == "USD" && max(p.BaseSalary.Value.MinValue, p.BaseSalary.Value.MaxValue) > builtinMaxHourlyUSD {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, period
}
