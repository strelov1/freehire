package sources

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
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
	return s.hydrate(ctx, c, nil, func(Job) bool { return false })
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
	return s.hydrate(ctx, c, gate.Covered(names), gate.Rejected)
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

// hydrate maps the candidates to Jobs, buying a posting page only for a posting the pipeline
// will keep, within the run's budget. Candidates arrive newest first (the window, then the
// backlog), so the budget goes to the freshest postings.
//
// A new posting whose page could not be read is DEFERRED rather than stored body-less: a
// stored row is seen, and once it ages past pipeline.HydrationRetryWindow nothing would buy
// its body again, while a deferred one is simply new on a later run.
//
// Once the edge refuses one request (403/429) the rest of the run's hydration is deferred too:
// the refusal is about the IP, and every further request would be refused alike and only
// prolong the penalty.
func (s startupjobs) hydrate(ctx context.Context, c startupjobsCrawl, covered map[string]bool, rejected func(Job) bool) ([]Job, error) {
	out := c.refreshes
	var toHydrate []Job
	deferred := 0
	for _, h := range c.candidates {
		base, ok := h.toJob()
		if !ok {
			continue
		}
		switch {
		case covered[base.Company] || rejected(base):
			out = append(out, base) // the pipeline discards it either way; a body would be pure loss
		case len(toHydrate) < s.maxNewPerRun:
			toHydrate = append(toHydrate, base)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("startupjobs: hydration budget %d reached; %d new postings wait for a later run", s.maxNewPerRun, deferred)
	}

	var refused atomic.Bool
	var failed atomic.Int64
	hydrated := fetchDetails(toHydrate, startupjobsDetailWorkers, func(j Job) (Job, bool) {
		if refused.Load() {
			return Job{}, false
		}
		body, err := s.body(ctx, j.URL)
		if err != nil {
			if isRateLimited(err) && !refused.Swap(true) {
				log.Printf("startupjobs: posting pages refused (%v); deferring the rest of this run's hydration", err)
			}
			if detailUnreadable(err) { // a 404/410 is the posting gone, which is an answer
				failed.Add(1)
			}
			return Job{}, false
		}
		j.Description = sanitizeHTML(body)
		return j, true
	})
	if n := failed.Load(); n > 0 {
		log.Printf("startupjobs: %d/%d posting pages could not be read; deferred to a later run", n, len(toHydrate))
	}
	// A run that needed bodies and could read none of them is a failure, not a quiet source —
	// see AGENTS.md's "a crawl that reads nothing of what it listed must fail". The Cloudflare
	// wall is exactly the failure this would otherwise hide behind a green unit. Postings that
	// were merely gone are an answer, not a failure.
	if failed.Load() > 0 && len(hydrated) == 0 {
		return nil, fmt.Errorf("startupjobs: needed %d posting pages and read none of them", len(toHydrate))
	}
	return append(out, hydrated...), nil
}

// body reads a posting page's schema.org JobPosting description. A page that answers without
// one is an error like any other unreadable page: the posting is deferred, not stored empty.
func (s startupjobs) body(ctx context.Context, url string) (string, error) {
	root, err := s.site.GetHTML(ctx, url)
	if err != nil {
		return "", err
	}
	var p startupjobsPosting
	if !ldJobPosting(root, &p) || strings.TrimSpace(p.Description) == "" {
		return "", fmt.Errorf("startupjobs: %s carries no JobPosting description", url)
	}
	return p.Description, nil
}

// startupjobsIndexFromHome reads the Algolia application, search key and posting index the
// site's own search uses from the homepage's <meta name="current-algolia-..."> tags. A missing
// tag is a markup change and must fail loudly, never fall back to a remembered key.
func startupjobsIndexFromHome(root *html.Node) (startupjobsIndex, error) {
	idx := startupjobsIndex{
		AppID:  metaName(root, "current-algolia-application-id"),
		APIKey: metaName(root, "current-algolia-api-key-search"),
		Index:  metaName(root, "current-algolia-index-post"),
	}
	if idx.AppID == "" || idx.APIKey == "" || idx.Index == "" {
		return startupjobsIndex{}, fmt.Errorf("startupjobs: homepage names no Algolia index (app %q, key set %v, index %q)",
			idx.AppID, idx.APIKey != "", idx.Index)
	}
	// The app id becomes a hostname and the index a path segment, so neither is trusted to be
	// anything but the plain identifier Algolia issues.
	if !startupjobsIdentifier.MatchString(idx.AppID) || !startupjobsIdentifier.MatchString(idx.Index) {
		return startupjobsIndex{}, fmt.Errorf("startupjobs: homepage names an implausible Algolia index (app %q, index %q)", idx.AppID, idx.Index)
	}
	return idx, nil
}

// metaName returns the content of the first <meta name="..."> with that name, or "".
func metaName(root *html.Node, name string) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "meta" && Attr(n, "name") == name {
			found = Attr(n, "content")
			return false
		}
		return true
	})
	return found
}

// sitemap reads every posting the sitemap lists and when the sitemap was generated — the
// OLDEST of its posting files' lastmod, so the window reaches back past the first file
// written. Any file failing fails the run: a missing file would read as a sixth of the
// catalogue gone.
func (s startupjobs) sitemap(ctx context.Context) ([]startupjobsSitemapPosting, time.Time, error) {
	index, err := getSitemap(ctx, s.feed, startupjobsSitemapIndex)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap index: %w", err)
	}
	var (
		out       []startupjobsSitemapPosting
		seen      = make(map[string]bool)
		generated time.Time
		files     int
	)
	for _, sm := range index.Sitemaps {
		if !startupjobsIsPostingSitemap(sm.Loc) {
			continue
		}
		files++
		mod := parseRFC3339(sm.LastMod)
		if mod == nil {
			return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap %s states no lastmod", sm.Loc)
		}
		if generated.IsZero() || mod.Before(generated) {
			generated = *mod
		}
		doc, err := getSitemap(ctx, s.feed, sm.Loc)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			id := startupjobsPostingID(u.Loc)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, startupjobsSitemapPosting{ID: id, URL: u.Loc})
		}
	}
	if files == 0 || len(out) == 0 {
		return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap index lists %d posting files carrying %d postings", files, len(out))
	}
	return out, generated, nil
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

// walk reads every posting created at or after since, in slices of created_at_i, newest
// first.
//
// created_at_i, not published_at_i, is the axis: published_at_i is re-stamped in daily batches
// at midnight UTC, so a one-second slice of it can hold far more than the 500 a query may read,
// and nothing narrower than a second exists. created_at_i is when the row was created and never
// moves, so a posting cannot migrate along it mid-walk.
//
// Each slice is [lower, upper); the first is open above, so a posting created during the walk
// is either read or created after the walk began. A slice whose nbHits exceeds the window is
// re-asked narrower; one that cannot be narrowed — more than 500 postings created in one
// second — fails the crawl, because reading its first 500 and calling it complete is exactly
// the silent truncation a source-scoped sweep turns into mass closure.
func (s startupjobs) walk(ctx context.Context, idx startupjobsIndex, since, now int64, budget *startupjobsBudget) ([]startupjobsHit, error) {
	var (
		out   []startupjobsHit
		seen  = make(map[string]bool)
		upper int64 // exclusive; 0 = open
		width = max(int64(1), (now-since)/4)
	)
	for {
		hi := now
		if upper != 0 {
			hi = upper
		}
		lower := max(hi-width, since)
		filters := []string{"created_at_i>=" + strconv.FormatInt(lower, 10)}
		if upper != 0 {
			filters = append(filters, "created_at_i<"+strconv.FormatInt(upper, 10))
		}

		first, err := s.page(ctx, idx, filters, 0, budget)
		if err != nil {
			return nil, err
		}
		if first.NbHits > startupjobsWindow {
			if hi-lower <= 1 {
				return nil, fmt.Errorf("startupjobs: %d postings created in the second at %d; Algolia reads at most %d per query and created_at_i cannot be narrowed further",
					first.NbHits, lower, startupjobsWindow)
			}
			width = narrowedWidth(hi-lower, first.NbHits)
			continue
		}
		hits, err := s.readSlice(ctx, idx, filters, first, budget)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if !seen[h.ObjectID] {
				seen[h.ObjectID] = true
				out = append(out, h)
			}
		}
		if lower <= since {
			return out, nil
		}
		upper = lower
		width = widenedWidth(hi-lower, first.NbHits)
	}
}

// readSlice reads every page of a slice that fits the window and proves it read all of it:
// the count must be exhaustive and the distinct hits must equal it. A slice that changed
// between its pages (a posting removed or added mid-read) is re-read once from the start
// before the mismatch is allowed to fail the crawl.
func (s startupjobs) readSlice(ctx context.Context, idx startupjobsIndex, filters []string,
	first startupjobsResult, budget *startupjobsBudget) ([]startupjobsHit, error) {
	for attempt := 0; ; attempt++ {
		if !first.ExhaustiveNbHits {
			return nil, fmt.Errorf("startupjobs: slice %v: Algolia reported an estimated count (%d), which cannot prove the slice was read whole",
				filters, first.NbHits)
		}
		var hits []startupjobsHit
		read := make(map[string]bool, first.NbHits)
		add := func(page []startupjobsHit) {
			for _, h := range page {
				if !read[h.ObjectID] {
					read[h.ObjectID] = true
					hits = append(hits, h)
				}
			}
		}
		add(first.Hits)
		for page := 1; page*startupjobsPageSize < first.NbHits; page++ {
			res, err := s.page(ctx, idx, filters, page, budget)
			if err != nil {
				return nil, err
			}
			add(res.Hits)
		}
		if len(hits) == first.NbHits {
			return hits, nil
		}
		if attempt == 1 {
			return nil, fmt.Errorf("startupjobs: slice %v: read %d distinct postings of the %d Algolia counted",
				filters, len(hits), first.NbHits)
		}
		var err error
		if first, err = s.page(ctx, idx, filters, 0, budget); err != nil {
			return nil, err
		}
		if first.NbHits > startupjobsWindow {
			return nil, fmt.Errorf("startupjobs: slice %v grew past the %d window while being read (%d)",
				filters, startupjobsWindow, first.NbHits)
		}
	}
}

// lookupBacklog reads the index metadata of the listed postings the catalogue has never
// stored and the window did not cover, as far as the budget the window left allows.
//
// The backlog is mostly postings judged before and discarded — non-technical, or from an
// employer crawled first-party — which are never stored, so they come back every run. Reading
// it newest-first would spend every run on the same freshly-discarded top and never reach the
// rest, so the run reads one chunk of it chosen at RANDOM. Not by the clock: the provider runs
// on a fixed cadence, and a chunk named by the hour would visit only every other chunk at a
// two-hour cadence whenever the chunk count is even. A new technical posting that fell through
// a gap in the window is found within a few cycles of len/chunk runs.
// An id the index no longer knows was removed since the sitemap was written, and is skipped.
func (s startupjobs) lookupBacklog(ctx context.Context, idx startupjobsIndex, backlog []string,
	budget *startupjobsBudget) ([]startupjobsHit, error) {
	queries := min(budget.left, startupjobsMaxBacklogQueries)
	if len(backlog) == 0 || queries <= 0 {
		return nil, nil
	}
	slices.SortFunc(backlog, func(a, b string) int { return startupjobsCompareIDs(b, a) })
	chunk := queries * startupjobsPageSize
	chunks := (len(backlog) + chunk - 1) / chunk
	start := s.pick(chunks) * chunk
	ids := backlog[start:min(start+chunk, len(backlog))]

	var out []startupjobsHit
	for len(ids) > 0 {
		n := min(len(ids), startupjobsPageSize)
		clauses := make([]string, n)
		for i, id := range ids[:n] {
			clauses[i] = "objectID:" + id
		}
		res, err := s.query(ctx, idx, startupjobsQuery{
			HitsPerPage:          startupjobsPageSize,
			Filters:              strings.Join(clauses, " OR "),
			AttributesToRetrieve: startupjobsHitAttributes,
		}, budget)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Hits...)
		ids = ids[n:]
	}
	return out, nil
}

// startupjobsCompareIDs orders posting ids numerically; the index issues them in creation order.
func startupjobsCompareIDs(a, b string) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}

// page asks Algolia for one page of one walk slice.
func (s startupjobs) page(ctx context.Context, idx startupjobsIndex, filters []string, page int, budget *startupjobsBudget) (startupjobsResult, error) {
	return s.query(ctx, idx, startupjobsQuery{
		HitsPerPage:          startupjobsPageSize,
		Page:                 page,
		NumericFilters:       filters,
		AttributesToRetrieve: startupjobsHitAttributes,
	}, budget)
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

// usable reports whether the index states enough for this hit to become a Job: an id to key
// it on, a path to link it to, a title, and the employer the catalogue files it under.
func (h startupjobsHit) usable() bool {
	return h.ObjectID != "" && strings.HasPrefix(h.Path, "/") && h.Title != "" && h.CompanyName != ""
}

// toJob maps an index hit to a body-less Job. PostedAt stays nil: the index's published_at
// is re-stamped in daily batches, and the page's datePosted is not a posting date either.
func (h startupjobsHit) toJob() (Job, bool) {
	if !h.usable() {
		return Job{}, false
	}
	job := Job{
		ExternalID:     h.ObjectID,
		URL:            startupjobsBaseURL + h.Path,
		Title:          h.Title,
		Company:        h.CompanyName,
		Location:       h.Location,
		WorkMode:       workplaceTypeMode(h.WorkplaceTypeID),
		EmploymentType: startupjobsEmploymentType(h.EmploymentTypeID),
		Countries:      countryFromCode(h.CountryCode),
	}
	job.Remote = job.WorkMode == "remote"
	h.applySalary(&job)
	return job, true
}

// applySalary maps the index's structured salary. The bounds are whole currency units, the
// currency is ISO 4217 and the interval names the period; an interval freehire's vocabulary has
// no value for (week, biweekly) drops the whole salary rather than filing an amount under the
// wrong period.
func (h startupjobsHit) applySalary(job *Job) {
	if h.SalaryCurrency == "" || (h.SalaryMin == nil && h.SalaryMax == nil) {
		return
	}
	period := startupjobsSalaryPeriod(h.SalaryInterval)
	if period == "" {
		return
	}
	var lo, hi *int
	if h.SalaryMin != nil {
		lo = roundSalaryPart(*h.SalaryMin)
	}
	if h.SalaryMax != nil {
		hi = roundSalaryPart(*h.SalaryMax)
	}
	if lo == nil && hi == nil {
		return
	}
	job.SalaryMin, job.SalaryMax = lo, hi
	job.SalaryCurrency, job.SalaryPeriod = strings.ToUpper(h.SalaryCurrency), period
}

// startupjobsSalaryPeriod maps the index's salary_interval onto vocab.SalaryPeriodValues; week
// and biweekly have no freehire period.
func startupjobsSalaryPeriod(interval string) string {
	switch interval {
	case "year", "month", "day", "hour":
		return interval
	default:
		return ""
	}
}

// startupjobsEmploymentType maps the index's employment_type_id — a closed four-value facet
// (full-time, contractor, part-time, internship) — onto vocab.EmploymentTypeValues.
func startupjobsEmploymentType(t string) string {
	switch t {
	case "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "contractor":
		return "contract"
	case "internship":
		return "internship"
	default:
		return ""
	}
}
