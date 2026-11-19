package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// glints adapts Glints (glints.com), the large Southeast Asian job platform (Indonesia, Vietnam,
// Singapore, Malaysia; ~80k public postings across every profession). It is boardless and an
// aggregator — the employer is read per posting. See providers/glints.md for the measurements
// behind each rule.
//
// The posting set comes from the job sitemaps (robots.txt disallows the explore search, and its
// GraphQL API answers an anonymous client a four-posting stub that ignores every filter and
// page), which carry nothing but each posting's URL. The whole catalogue is crawled and the
// pipeline's own catalogue filter decides what is stored. Every fact is bought with the posting
// page: the adapter hydrates only postings the catalogue lacks and refreshes the rest by identity.
//
// glints.com answers a plain Go client 403, so production crawls it through the Scrapling tier's
// impersonating fetcher (scraplingtier.go).
type glints struct {
	http         glintsHTTP
	limiter      waiter
	maxNewPerRun int
	// now dates the backlog window; nil is time.Now.
	now func() time.Time
}

// glintsHTTP is the transport the adapter needs: the sitemaps as XML and each posting page.
type glintsHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	glintsSitemapIndex = "https://glints.com/sitemap_index.xml"
	// glintsRequestInterval paces every request of a run, shard and posting page alike, through
	// one limiter. A page took ~1-2 s through the impersonating fetcher and 8 in a row drew no
	// refusal; two a second is courtesy toward a large site, not a limit we measured.
	glintsRequestInterval = 500 * time.Millisecond
	// glintsWorkers keeps enough requests in flight that the limiter, not latency, sets the pace.
	glintsWorkers = 4
	// glintsShardAttempts is how often one sitemap shard is asked for before the crawl fails: the
	// listing is the whole catalogue or an error, and ~870 shards make a single refusal likely.
	glintsShardAttempts = 3
	// glintsMaxNewPerRun bounds how many unseen postings one run hydrates. Through the Scrapling
	// tier a page took ~2.4 s on the crawl host (the first production run, 2026-11-17, read 1,076
	// pages before its 60-minute deadline), so 1200 pages are ~48 minutes with the ~7 minutes the
	// shards take, inside the unit's hour. The first runs face the whole
	// ~80k catalogue, taken in slices; the pipeline stores every posting it is handed, so what is
	// hydrated leaves the unseen set. The backlog lane (see hydrationLanes) is only for pages
	// that failed to read, which would otherwise sit at the head of the queue.
	glintsMaxNewPerRun = 1200
	glintsBacklogLane  = 500
	// glintsITCategory is the id of Glints' own top-level category "Computer & Software", the
	// one structured statement of IT-ness a posting carries.
	glintsITCategory = "9dd9bc4d-38d5-4b5e-876a-de19bc22da45"
)

// NewGlints builds the adapter over the given client.
func NewGlints(c glintsHTTP) Source {
	return glints{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(glintsRequestInterval), 1),
		maxNewPerRun: glintsMaxNewPerRun,
	}
}

func (glints) Provider() string { return "glints" }

// boardless marks glints as having no per-tenant board id: one crawl covers the whole site.
func (glints) Boardless() {}

// aggregator keeps glints in the source facet: one crawl aggregates postings from many
// companies, so filtering by source=glints is meaningful.
func (glints) Aggregator() {}

// fullCatalog: the listing reads the sitemap index and every job shard, and fails on any shard
// still unreadable after its attempts, or on a listing with no posting in it, so a
// finished crawl has seen every posting the site lists. See the fullCatalog interface (source.go).
func (glints) FullCatalog() {}

var (
	// glintsShardRE selects the job sitemaps (sitemap_job_<country>_<n>.xml) from the index; the
	// sitemap_sourced_job_ family lists /opportunities/s/ pages, which are copies of JobStreet
	// postings (their externalApplyURL is a jobstreet.com page) and are not read.
	glintsShardRE = regexp.MustCompile(`^https://glints\.com/sitemap_job_[a-z]{2}_\d+\.xml$`)
	// glintsPostingRE matches a posting page: /<country>/opportunities/jobs/<title-slug>/<uuid>.
	// The locale twin of every URL (/<country>/en/opportunities/...) does not match, which is
	// how the listing keeps one entry per posting.
	glintsPostingRE = regexp.MustCompile(`^https://glints\.com/[a-z]{2}/opportunities/jobs/([^/]+)/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)
)

// glintsLink is one posting the sitemaps name.
type glintsLink struct{ ID, URL string }

// Fetch hydrates every listed posting: the path for callers that supply no seen set. FetchNew is
// the one ingest uses.
func (g glints) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return g.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the whole sitemap and hydrates only what the catalogue lacks: a seen posting is
// a liveness refresh with no request of its own, and new ones are hydrated up to the per-run
// budget. A new posting past the budget is deferred rather than stored half-read.
func (g glints) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	links, err := g.listing(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []glintsLink
	for _, l := range links {
		if seen(l.ID) {
			jobs = append(jobs, Job{ExternalID: l.ID, URL: l.URL, SeenRefresh: true})
		} else {
			fresh = append(fresh, l)
		}
	}
	now := time.Now
	if g.now != nil {
		now = g.now
	}
	budget := g.maxNewPerRun
	if budget <= 0 {
		budget = glintsMaxNewPerRun
	}
	backlog := min(glintsBacklogLane, budget)
	queue := hydrationLanes(fresh, budget-backlog, backlog, now())
	if deferred := len(fresh) - len(queue); deferred > 0 {
		log.Printf("glints: hydrating %d of %d new postings; %d deferred to a later run", len(queue), len(fresh), deferred)
	}
	var wanted, read atomic.Int64
	hydrated := fetchDetails(queue, glintsWorkers, func(l glintsLink) (Job, bool) {
		wanted.Add(1)
		job, ok := g.detail(ctx, l)
		if ok {
			read.Add(1)
		}
		return job, ok
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("glints: listed %d new postings and read none of them", wanted.Load())
	}
	return append(jobs, hydrated...), nil
}

// listing reads the sitemap index and every job shard and returns every posting. A
// shard that cannot be read, or one that lists no posting at all, fails it: glints is swept by
// source, so a listing missing a shard would close every posting in it.
func (g glints) listing(ctx context.Context) ([]glintsLink, error) {
	index, err := g.sitemap(ctx, glintsSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("glints: sitemap index: %w", err)
	}
	var shards []string
	for _, s := range index.Sitemaps {
		if glintsShardRE.MatchString(strings.TrimSpace(s.Loc)) {
			shards = append(shards, strings.TrimSpace(s.Loc))
		}
	}
	if len(shards) == 0 {
		return nil, fmt.Errorf("glints: sitemap index names no job shard (%d entries)", len(index.Sitemaps))
	}
	type shardResult struct {
		links []glintsLink
		err   error
	}
	results := make([]shardResult, len(shards))
	sem := make(chan struct{}, glintsWorkers)
	var wg sync.WaitGroup
	for i := range shards {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			doc, err := g.sitemap(ctx, shards[i])
			if err != nil {
				results[i].err = fmt.Errorf("glints: shard %s: %w", shards[i], err)
				return
			}
			for _, u := range doc.URLs {
				m := glintsPostingRE.FindStringSubmatch(strings.TrimSpace(u.Loc))
				if m == nil {
					continue
				}
				results[i].links = append(results[i].links, glintsLink{ID: m[2], URL: strings.TrimSpace(u.Loc)})
			}
		}()
	}
	wg.Wait()
	var links []glintsLink
	for i, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		// A shard with no posting in it is not an empty slice of the site but a page other than
		// the one we read (a challenge, a moved layout).
		if len(r.links) == 0 {
			return nil, fmt.Errorf("glints: shard %s listed no job posting", shards[i])
		}
		links = append(links, r.links...)
	}
	log.Printf("glints: %d shards list %d postings", len(shards), len(links))
	return links, nil
}

// sitemap fetches one sitemap document through the limiter, asking up to glintsShardAttempts
// times.
func (g glints) sitemap(ctx context.Context, url string) (sitemapDoc, error) {
	var err error
	for range glintsShardAttempts {
		if err = g.limiter.Wait(ctx); err != nil {
			return sitemapDoc{}, err
		}
		var doc sitemapDoc
		if doc, err = getSitemap(ctx, g.http, url); err == nil {
			return doc, nil
		}
	}
	return sitemapDoc{}, err
}

// glintsLD is the slice of the page's schema.org JobPosting the adapter reads: the HTML body,
// which the page's state carries only as an editor document.
type glintsLD struct {
	Description string `json:"description"`
}

// glintsPage is the slice of the page's __NEXT_DATA__ the adapter reads.
type glintsPage struct {
	Props struct {
		PageProps struct {
			InitialData struct {
				Data glintsPosting `json:"data"`
			} `json:"initialData"`
		} `json:"pageProps"`
	} `json:"props"`
}

type glintsPosting struct {
	ID                    string  `json:"id"`
	Title                 string  `json:"title"`
	Status                string  `json:"status"`
	CreatedAt             string  `json:"createdAt"`
	ExpiryDate            string  `json:"expiryDate"`
	ClosedAt              *string `json:"closedAt"`
	Type                  string  `json:"type"`
	WorkArrangementOption string  `json:"workArrangementOption"`
	IsRemote              bool    `json:"isRemote"`
	MinYearsOfExperience  *int    `json:"minYearsOfExperience"`
	ShouldShowSalary      bool    `json:"shouldShowSalary"`
	ExternalApplyURL      *string `json:"externalApplyURL"`
	Company               struct {
		Name string `json:"name"`
	} `json:"company"`
	Country struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"country"`
	Location glintsLocation `json:"location"`
	Salaries []struct {
		SalaryType   string `json:"salaryType"`
		SalaryMode   string `json:"salaryMode"`
		MinAmount    *int   `json:"minAmount"`
		MaxAmount    *int   `json:"maxAmount"`
		CurrencyCode string `json:"CurrencyCode"`
	} `json:"salaries"`
	Category glintsCategory `json:"hierarchicalJobCategory"`
}

// glintsLocation is a place and its ancestors: district, city, province, country (levels 4..1).
type glintsLocation struct {
	FormattedName string           `json:"formattedName"`
	Level         int              `json:"level"`
	Parents       []glintsLocation `json:"parents"`
}

type glintsCategory struct {
	ID      string `json:"id"`
	Level   int    `json:"level"`
	Parents []struct {
		ID    string `json:"id"`
		Level int    `json:"level"`
	} `json:"parents"`
}

// detail fetches one posting page and maps it to a Job, returning ok=false when the page cannot
// be read, is gone, is no longer open, carries no body or no employer — so the caller skips just
// that posting and a later run asks again.
func (g glints) detail(ctx context.Context, l glintsLink) (Job, bool) {
	if err := g.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, err := g.http.GetHTML(ctx, l.URL)
	if err != nil {
		if detailUnreadable(err) {
			log.Printf("glints: posting %s failed; deferring: %v", l.ID, err)
		}
		return Job{}, false
	}
	job, ok := glintsJob(root, l)
	if !ok {
		log.Printf("glints: posting %s carries no usable posting; deferring", l.ID)
	}
	return job, ok
}

func glintsJob(root *html.Node, l glintsLink) (Job, bool) {
	var page glintsPage
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" || json.Unmarshal([]byte(raw), &page) != nil {
		return Job{}, false
	}
	p := page.Props.PageProps.InitialData.Data
	var ld glintsLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false
	}
	title := strings.Join(strings.Fields(p.Title), " ")
	company := strings.Join(strings.Fields(p.Company.Name), " ")
	description := sanitizeHTML(ld.Description)
	if p.ID != l.ID || p.Status != "OPEN" || p.ClosedAt != nil || title == "" || company == "" ||
		strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	workMode := workplaceTypeMode(p.WorkArrangementOption)
	if p.IsRemote {
		workMode = "remote"
	}
	job := Job{
		ExternalID:          l.ID,
		URL:                 l.URL,
		Title:               title,
		Company:             company,
		Location:            p.glintsLocationLine(),
		Description:         description,
		Remote:              workMode == "remote",
		WorkMode:            workMode,
		PostedAt:            NotFuture(parseRFC3339(p.CreatedAt)),
		ApplicationDeadline: glintsDeadline(p.ExpiryDate),
		EmploymentType:      glintsEmploymentTypes[p.Type],
		Countries:           countryFromCode(p.Country.Code),
		ExperienceYearsMin:  p.MinYearsOfExperience,
		IsTechHint:          p.Category.isIT(),
	}
	p.salary(&job)
	return job, true
}

// glintsDeadline reads the posting's expiry as a UTC day: the site states it either as a bare date
// or as that date at midnight. It is not passed through NotFuture, which is for a posted_at and
// would drop every deadline more than two days away.
func glintsDeadline(s string) *time.Time {
	if len(s) < len("2006-01-02") {
		return nil
	}
	t, err := time.Parse("2006-01-02", s[:len("2006-01-02")])
	if err != nil {
		return nil
	}
	return &t
}

// glintsEmploymentTypes maps the site's job type enum. PROJECT_BASED is a fixed-scope contract.
var glintsEmploymentTypes = map[string]string{
	"FULL_TIME":     "full_time",
	"PART_TIME":     "part_time",
	"CONTRACT":      "contract",
	"PROJECT_BASED": "contract",
	"INTERNSHIP":    "internship",
}

// isIT reports whether the posting is filed under Glints' "Computer & Software" category, at
// whatever level the posting's own category sits.
func (c glintsCategory) isIT() bool {
	if c.ID == glintsITCategory {
		return true
	}
	for _, p := range c.Parents {
		if p.ID == glintsITCategory {
			return true
		}
	}
	return false
}

// glintsLocationLine names the posting's city, province and country. The district (level 4) is
// left out: it is a neighbourhood the geography dictionary cannot place and it crowds out the
// city it sits in.
func (p glintsPosting) glintsLocationLine() string {
	var names []string
	for l := &p.Location; l != nil; {
		if l.Level >= 2 && l.Level <= 3 {
			names = append(names, strings.TrimSpace(l.FormattedName))
		}
		if len(l.Parents) == 0 {
			break
		}
		l = &l.Parents[0]
	}
	return distinctJoin(append(names, strings.TrimSpace(p.Country.Name)), ", ", func(s string) string { return s })
}

// glintsSalaryPeriods maps the site's salary mode to the period vocabulary.
var glintsSalaryPeriods = map[string]string{
	"YEAR": "year", "MONTH": "month", "DAY": "day", "HOUR": "hour",
}

// salary takes the basic pay the posting displays. A posting that hides its salary lists none,
// and one in a mode outside the table is left without rather than read as a monthly figure.
func (p glintsPosting) salary(job *Job) {
	if !p.ShouldShowSalary {
		return
	}
	for _, s := range p.Salaries {
		period := glintsSalaryPeriods[s.SalaryMode]
		if s.SalaryType != "BASIC" || period == "" || s.CurrencyCode == "" {
			continue
		}
		lo, hi := s.MinAmount, s.MaxAmount
		if (lo == nil || *lo <= 0) && (hi == nil || *hi <= 0) {
			continue
		}
		if lo != nil && *lo > 0 {
			job.SalaryMin = lo
		}
		if hi != nil && *hi > 0 {
			job.SalaryMax = hi
		}
		job.SalaryCurrency, job.SalaryPeriod = s.CurrencyCode, period
		return
	}
}
