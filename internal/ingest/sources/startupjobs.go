package sources

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// startupjobs adapts startup.jobs, which despite its name is not a startup board: it re-lists
// corporate ATS postings beside the startups. It is an aggregator in every sense the pipeline
// cares about (many employers, read per posting, most already crawled first-party), so it is
// boardless, carries the aggregator marker, and implements CoverageGated.
//
// Three hosts, each read for what only it has, and each with its own budget:
//
//   - The SITEMAP (cdn.startup.jobs, no Cloudflare challenge) lists every live posting's URL,
//     and the posting id ends it. It is regenerated once a day (03:00 UTC), and it is the only
//     affordable statement of the WHOLE catalogue — which is what liveness and the sweep need.
//     It names no title or employer, so it refreshes what is stored and nothing else.
//   - The CATALOGUE is a public Algolia index (Post_production): title, employer, place,
//     workplace type, employment type, salary — everything but the body. Its search key allows
//     about 100 queries per IP per hour and a query returns at most 100 hits, so the index cannot
//     be read whole in a run. The adapter reads only what it needs from it: the
//     WINDOW — the postings created in the last startupjobsWindowSpan, or since an hour before
//     the sitemap was generated if that is sooner — and a randomly chosen chunk of the postings
//     the catalogue has never stored. Its credentials are the homepage's <meta> tags, read
//     every run because the search key is a SECURED key whose validUntil is a few hours out.
//   - The BODY is only on the posting page, behind a Cloudflare JS challenge that only the
//     Chrome-fingerprint transport passes, and which starts refusing an IP after a long run of
//     requests. It is spent on nothing the pipeline is going to throw away (FetchNewGated).
//
// fullCatalog — the sweep closing by source whatever goes unrefreshed for 48 hours — holds
// because every stored posting is refreshed by every run until it leaves the site: by the
// window while it is younger than startupjobsWindowSpan, by the sitemap from the first
// generation after it was created, and the gap between the two (a posting created just after a
// generation, out of the window six hours later, in the next sitemap within a day) stays far
// inside 48 hours as long as the sitemap is at most startupjobsSitemapMaxAge old — which the run
// checks. A run that cannot produce both whole fails rather than refreshing less: any sitemap
// file or Algolia query failing, a stale sitemap, a sitemap well short of the index's own
// count, a window slice that cannot be read to its exhaustive count (see walk). A failed run
// ingests nothing, and a provider that ingested nothing is not swept at all.
type startupjobs struct {
	// site reaches the Cloudflare-fronted pages (the homepage's Algolia meta tags and each
	// posting page); it must be the Chrome-fingerprint transport.
	site HTMLGetter
	// feed reaches the sitemap CDN and search reaches Algolia; neither has Cloudflare in front,
	// and both take the shared client.
	feed   XMLGetter
	search HeaderJSONPoster
	// maxNewPerRun bounds how many posting pages one run hydrates. Anything past it is
	// deferred — not yielded, so not stored — and is offered again by a later run.
	maxNewPerRun int
	now          func() time.Time
	// pick chooses which of n backlog chunks a run reads (see lookupBacklog).
	pick func(n int) int
}

const (
	startupjobsBaseURL = "https://startup.jobs"
	// startupjobsSitemapIndex is the sitemap index robots.txt names. Despite the .gz suffix
	// it and its files are served as plain XML.
	startupjobsSitemapIndex = "https://cdn.startup.jobs/sitemaps/startupjobs/sitemap.xml.gz"
	// startupjobsAlgoliaURL is Algolia's query endpoint for one index; the -dsn host is the
	// read replica the site's own search uses.
	startupjobsAlgoliaURL = "https://%s-dsn.algolia.net/1/indexes/%s/query"
	// startupjobsWindow is the index's paginationLimitedTo: past the 500th hit Algolia answers
	// 400 "you can only fetch the 500 hits for this query". A slice is readable only when its
	// nbHits fits this.
	startupjobsWindow = 500
	// startupjobsPageSize is what a query is answered with: hitsPerPage — and length — above
	// 100 are silently clamped to 100.
	startupjobsPageSize = 100
	// startupjobsQueryBudget is how many Algolia queries one run may spend. The key allows about
	// 100 an hour per IP and the provider runs every two hours. The window takes what it needs
	// first; the backlog lookup takes what is left, up to its cap.
	startupjobsQueryBudget = 80
	// startupjobsWindowSpan is how far back the window reaches. Wide enough that every posting
	// is read by several two-hourly runs while it is new; narrow enough that the window never
	// competes with the budget. Reaching back to the sitemap instead would cost ~90 queries in
	// the hours before 03:00, the whole budget and more.
	startupjobsWindowSpan = 6 * time.Hour
	// startupjobsMaxBacklogQueries caps the backlog's share, so a cheap window early in the day
	// does not spend the whole budget on postings that have waited weeks already.
	startupjobsMaxBacklogQueries = 40
	// startupjobsSitemapOverlap is how far before a fresh sitemap's generation the window
	// reaches, so a posting created while the sitemap was being written is read by one or the
	// other.
	startupjobsSitemapOverlap = time.Hour
	// startupjobsSitemapMaxAge is how stale the sitemap may be before the run refuses it. A
	// stored posting created after the sitemap is refreshed only by the window, and once it
	// leaves the window only the NEXT sitemap refreshes it; refusing a sitemap older than this
	// keeps that gap well inside the sweep's 48 hours (see the type comment).
	startupjobsSitemapMaxAge = 36 * time.Hour
	// startupjobsMinSitemapShare is how much of the index's own count of pre-window postings the
	// sitemap must list. The two legitimately differ by the postings created and removed since the
	// sitemap was written (a few percent); a generation that lost a file loses a sixth of the
	// catalogue, and source-sweeping on it would close that sixth.
	startupjobsMinSitemapShare = 0.95
	// startupjobsTargetHits is what the adaptive slice width aims at: under the window with
	// room for the density to vary inside it, so a slice rarely has to be re-asked narrower.
	startupjobsTargetHits = 400
	// startupjobsDefaultMaxNewPerRun is the per-run hydration budget. Steady state needs far less;
	// a cold start accretes.
	startupjobsDefaultMaxNewPerRun = 1000
	// startupjobsDetailWorkers is the posting-page fan-out. The pacer, not the pool, bounds the
	// rate; two workers keep its tokens from idling behind one slow response.
	startupjobsDetailWorkers = 2
	// startupjobsDetailInterval paces the Cloudflare-fronted requests at ~1 req/s; the edge is known
	// to start refusing an IP after a long run, so the pace stays low.
	startupjobsDetailInterval = time.Second
	startupjobsDetailBurst    = 1
)

// startupjobsIdentifier is the shape of an Algolia application id or index name.
var startupjobsIdentifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// startupjobsPostingPath is a posting's path on the site: a slug ending in the posting id.
var startupjobsPostingPath = regexp.MustCompile(`^/[^/]+-(\d+)$`)

// startupjobsHitAttributes is every field the adapter maps — asking for only these keeps a
// 100-hit page small and makes the dependency on the index's schema explicit.
var startupjobsHitAttributes = []string{
	"objectID", "path", "title", "company_name", "location", "country_code",
	"workplace_type_id", "employment_type_id", "salary_min", "salary_max",
	"salary_currency", "salary_interval", "created_at_i",
}

// NewStartupJobs builds the startup.jobs adapter. site must be the Chrome-fingerprint
// transport (Cloudflare challenges every other one); feed and search are the shared client.
// A non-positive maxNewPerRun takes the default.
func NewStartupJobs(site HTMLGetter, feed XMLGetter, search HeaderJSONPoster, maxNewPerRun int) Source {
	if maxNewPerRun <= 0 {
		maxNewPerRun = startupjobsDefaultMaxNewPerRun
	}
	return startupjobs{site: site, feed: feed, search: search, maxNewPerRun: maxNewPerRun, now: time.Now, pick: rand.N[int]}
}

// pacedStartupJobsSite paces every Cloudflare-fronted request one run makes through one
// limiter: the homepage read and the posting pages share the IP the edge is counting.
func pacedStartupJobsSite(c HTMLGetter) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(startupjobsDetailInterval), startupjobsDetailBurst),
	}
}

func (startupjobs) Provider() string { return "startupjobs" }

// startup.jobs is one public catalogue with no per-tenant board.
func (startupjobs) boardless() {}

// It re-lists many employers' postings, most of them from ATSes freehire crawls directly.
func (startupjobs) aggregator() {}

// Every run lists the whole catalogue or fails; see the type comment.
func (startupjobs) fullCatalog() {}

// startupjobsIndex is what the homepage's meta tags name: which Algolia application, with
// which search key, holds the postings.
type startupjobsIndex struct {
	AppID  string
	APIKey string
	Index  string
}

// startupjobsHit is one posting as the Algolia index states it. Every field but the body.
type startupjobsHit struct {
	ObjectID         string   `json:"objectID"`
	Path             string   `json:"path"`
	Title            string   `json:"title"`
	CompanyName      string   `json:"company_name"`
	Location         string   `json:"location"`
	CountryCode      string   `json:"country_code"`
	WorkplaceTypeID  string   `json:"workplace_type_id"`
	EmploymentTypeID string   `json:"employment_type_id"`
	SalaryMin        *float64 `json:"salary_min"`
	SalaryMax        *float64 `json:"salary_max"`
	SalaryCurrency   string   `json:"salary_currency"`
	SalaryInterval   string   `json:"salary_interval"`
	CreatedAt        int64    `json:"created_at_i"`
}

// startupjobsQuery is the subset of Algolia's query parameters the adapter sets.
type startupjobsQuery struct {
	Query                 string   `json:"query"`
	HitsPerPage           int      `json:"hitsPerPage"`
	Page                  int      `json:"page"`
	NumericFilters        []string `json:"numericFilters,omitempty"`
	Filters               string   `json:"filters,omitempty"`
	AttributesToRetrieve  []string `json:"attributesToRetrieve"`
	AttributesToHighlight []string `json:"attributesToHighlight"`
	// Analytics off: the crawl is not a visitor's search and must not skew the site's own
	// search analytics.
	Analytics bool `json:"analytics"`
}

// startupjobsResult is the subset of Algolia's answer the adapter reads. ExhaustiveNbHits is
// load-bearing: on a large filtered result Algolia may report an ESTIMATED nbHits (flagged
// false), and a walk that trusted one would verify a slice against the wrong number.
type startupjobsResult struct {
	Hits             []startupjobsHit `json:"hits"`
	NbHits           int              `json:"nbHits"`
	ExhaustiveNbHits bool             `json:"exhaustiveNbHits"`
}

// startupjobsPosting is the subset of the posting page's schema.org JobPosting the adapter
// reads. Only the body: datePosted is not the posting date (live postings carry years-old dates)
// and validThrough is stamped a month ahead of every crawl, so neither says anything about
// the posting's lifecycle.
type startupjobsPosting struct {
	Description string `json:"description"`
}

// startupjobsSitemapPosting is one posting the sitemap lists.
type startupjobsSitemapPosting struct {
	ID  string
	URL string
}

// startupjobsCrawl is one run's reading of the catalogue: the stored postings it saw, as
// liveness refreshes, and the postings the catalogue does not hold, with their index metadata.
type startupjobsCrawl struct {
	refreshes  []Job
	candidates []startupjobsHit
}

// startupjobsBudget counts one run's Algolia queries.
type startupjobsBudget struct{ left int }

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: every
// posting the run read metadata for, body-less.
func (s startupjobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	c, err := s.crawl(ctx, func(string) bool { return false })
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(c.candidates))
	for _, h := range c.candidates {
		if job, ok := h.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew hydrates the new postings within the run's budget. It is the path taken when the
// coverage gate does not apply; FetchNewGated is the one production takes.
func (s startupjobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	c, err := s.crawl(ctx, seen)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, c, nil)
}

// FetchNewGated is FetchNew told what the pipeline is going to discard, and it matters more
// here than on any other aggregator. A discarded posting is never stored, so it is never seen,
// so without the gate the crawl would buy — through the one transport that has a budget — the
// body of every non-technical posting and of every posting whose employer freehire already
// crawls first-party, on every run. Such a posting is still yielded, body-less, so the
// pipeline makes and counts the decision.
func (s startupjobs) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	c, err := s.crawl(ctx, seen)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(c.candidates))
	for _, h := range c.candidates {
		if h.usable() {
			names = append(names, h.CompanyName)
		}
	}
	return s.hydrate(ctx, c, gate.Covered(names))
}

// crawl reads the run's view of the catalogue: the sitemap for every live posting, the Algolia
// window for the newest ones, and a randomly chosen chunk of the never-stored backlog for
// whatever query budget the window left.
func (s startupjobs) crawl(ctx context.Context, seen func(string) bool) (startupjobsCrawl, error) {
	root, err := s.site.GetHTML(ctx, startupjobsBaseURL+"/")
	if err != nil {
		return startupjobsCrawl{}, fmt.Errorf("startupjobs: homepage: %w", err)
	}
	idx, err := startupjobsIndexFromHome(root)
	if err != nil {
		return startupjobsCrawl{}, err
	}
	listed, generated, err := s.sitemap(ctx)
	if err != nil {
		return startupjobsCrawl{}, err
	}
	now := s.now()
	if age := now.Sub(generated); age > startupjobsSitemapMaxAge {
		return startupjobsCrawl{}, fmt.Errorf("startupjobs: sitemap generated %s ago (%s), past the %s the window may span",
			age.Round(time.Minute), generated.Format(time.RFC3339), startupjobsSitemapMaxAge)
	}

	budget := &startupjobsBudget{left: startupjobsQueryBudget}
	since := max(generated.Add(-startupjobsSitemapOverlap).Unix(), now.Add(-startupjobsWindowSpan).Unix())
	window, err := s.walk(ctx, idx, since, now.Unix(), budget)
	if err != nil {
		return startupjobsCrawl{}, err
	}
	total, err := s.query(ctx, idx, startupjobsQuery{HitsPerPage: 0}, budget)
	if err != nil {
		return startupjobsCrawl{}, err
	}
	if !total.ExhaustiveNbHits {
		return startupjobsCrawl{}, fmt.Errorf("startupjobs: Algolia estimated the index's size (%d), so the sitemap cannot be checked against it", total.NbHits)
	}
	if older := total.NbHits - len(window); float64(len(listed)) < startupjobsMinSitemapShare*float64(older) {
		return startupjobsCrawl{}, fmt.Errorf("startupjobs: sitemap lists %d postings where the index holds %d older than the window; refusing a sitemap that short",
			len(listed), older)
	}

	var c startupjobsCrawl
	inWindow := make(map[string]bool, len(window))
	for _, h := range window {
		inWindow[h.ObjectID] = true
		if seen(h.ObjectID) {
			if job, ok := h.toJob(); ok {
				job.SeenRefresh = true
				c.refreshes = append(c.refreshes, job)
			}
			continue
		}
		c.candidates = append(c.candidates, h)
	}
	var backlog []string
	for _, p := range listed {
		switch {
		case inWindow[p.ID]:
		case seen(p.ID):
			// The sitemap names no title or employer, and a refresh needs neither: the
			// pipeline refreshes by identity and judges it on the stored row's evidence.
			c.refreshes = append(c.refreshes, Job{ExternalID: p.ID, URL: p.URL, SeenRefresh: true})
		default:
			backlog = append(backlog, p.ID)
		}
	}
	found, err := s.lookupBacklog(ctx, idx, backlog, budget)
	if err != nil {
		return startupjobsCrawl{}, err
	}
	c.candidates = append(c.candidates, found...)
	// One line a run, because the query budget is the thing that will need tuning and nothing
	// else records how it was spent.
	log.Printf("startupjobs: sitemap %d postings (generated %s), window %d, backlog read %d of %d unstored; %d of %d Algolia queries spent",
		len(listed), generated.Format(time.RFC3339), len(window), len(found), len(backlog),
		startupjobsQueryBudget-budget.left, startupjobsQueryBudget)
	return c, nil
}

// startupjobsIsPostingSitemap reports whether a sitemap index entry is one of the posting
// files (posts.xml.gz, posts1.xml.gz, ...) rather than companies, roles, locations, ...
func startupjobsIsPostingSitemap(loc string) bool {
	u, err := url.Parse(loc)
	if err != nil {
		return false
	}
	base := u.Path[strings.LastIndex(u.Path, "/")+1:]
	return strings.HasPrefix(base, "posts")
}

// startupjobsPostingID returns the posting id a startup.jobs posting URL ends in, or "".
func startupjobsPostingID(loc string) string {
	u, err := url.Parse(strings.TrimSpace(loc))
	if err != nil || u.Host != "startup.jobs" {
		return ""
	}
	m := startupjobsPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return ""
	}
	return m[1]
}

// startupjobsCompareIDs orders posting ids numerically; the index issues them in creation order.
func startupjobsCompareIDs(a, b string) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}

// query runs one Algolia query against the run's budget. Any failure fails the crawl: there is
// no partial result a fullCatalog crawl may return.
func (s startupjobs) query(ctx context.Context, idx startupjobsIndex, q startupjobsQuery, budget *startupjobsBudget) (startupjobsResult, error) {
	if budget.left <= 0 {
		return startupjobsResult{}, fmt.Errorf("startupjobs: run spent its %d Algolia queries (the search key allows ~100 an hour per IP) before reading the window", startupjobsQueryBudget)
	}
	budget.left--
	if q.AttributesToRetrieve == nil {
		q.AttributesToRetrieve = []string{"objectID"}
	}
	q.AttributesToHighlight = []string{}
	headers := map[string]string{
		"X-Algolia-Application-Id": idx.AppID,
		"X-Algolia-API-Key":        idx.APIKey,
	}
	var res startupjobsResult
	endpoint := fmt.Sprintf(startupjobsAlgoliaURL, strings.ToLower(idx.AppID), idx.Index)
	if err := s.search.PostJSONWithHeaders(ctx, endpoint, headers, q, &res); err != nil {
		return startupjobsResult{}, fmt.Errorf("startupjobs: algolia %v%s page %d: %w", q.NumericFilters, q.Filters, q.Page, err)
	}
	return res, nil
}

// narrowedWidth sizes the retry of a slice that overflowed the window: proportionally to the
// density it measured, and at least halved so a skewed slice still converges.
func narrowedWidth(width int64, hits int) int64 {
	next := min(width*startupjobsTargetHits/int64(hits), width/2)
	return max(next, 1)
}

// widenedWidth sizes the next slice from the density this one measured, growing at most
// fourfold so one sparse stretch cannot fling the next slice across a dense one.
func widenedWidth(width int64, hits int) int64 {
	next := width * 4
	if hits > 0 {
		next = min(next, width*startupjobsTargetHits/int64(hits))
	}
	return max(next, 1)
}
