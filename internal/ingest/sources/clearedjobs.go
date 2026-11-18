package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// clearedjobs adapts ClearedJobs.net (clearedjobs.net), the US job board for people holding a
// government security clearance: ~11.5k open postings from defense contractors and agencies,
// every trade the cleared workforce does, a large share of it IT. It is boardless and an
// aggregator — the employer is read per posting. See the "ClearedJobs traps" section of AGENTS.md
// for the measurements behind each rule.
//
// Its edge answers 403 to a plain Go client and serves Chrome's TLS fingerprint, so the whole
// crawl goes through the Scrapling tier's impersonating fetcher (see scraplingProviders). Two
// public surfaces name the postings, because neither names them all: the "active jobs" sitemap
// carries 10,000 of them, oldest first, and stops there — the newest ~1.5k are missing from it —
// while the search listing's deepest page (40 pages of 25 rows, cumulative) is the newest 1,000.
// Their union covers ~96% of the board. Every fact is bought with the posting page's schema.org
// JobPosting, only for postings the catalogue lacks; the rest are refreshed by identity.
//
// The clearance level the page states in its JobPosting `industry` field ("Top Secret / SCI +
// Polygraph") is not in the description text, so it is appended to the body as a labelled
// "Security Clearance:" line, which the clearance dictionary (internal/dict/location) reads into
// requires_clearance and clearance_levels the same way it reads an employer's own label.
type clearedjobs struct {
	http clearedjobsHTTP
	// limiter paces every request of a run, sitemap, listing and posting page alike.
	limiter waiter
	// freshPerRun and backlogPerRun bound one run's posting pages; see clearedjobsFreshPerRun.
	freshPerRun   int
	backlogPerRun int
	// deadlineMargin is how much of the run's deadline hydration leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNew returns, so one that runs into its deadline
	// stores nothing at all. Pages still unread when the margin is reached wait for a later run.
	deadlineMargin time.Duration
	// retryBackoff is the first pause before a refused request is asked again; it doubles.
	retryBackoff time.Duration
	now          func() time.Time
}

type clearedjobsHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	clearedjobsHost    = "clearedjobs.net"
	clearedjobsBaseURL = "https://" + clearedjobsHost
	// clearedjobsSitemapURL is the sitemap index; the shards worth reading are named
	// sitemap_active_jobs[_N].xml (the rest are search, career-guide and company pages).
	clearedjobsSitemapURL = clearedjobsBaseURL + "/sitemap.xml"
	// clearedjobsShardMarker selects the job shards out of the index.
	clearedjobsShardMarker = "/sitemap_active_jobs"
	// clearedjobsNewestURL is the search listing's deepest page: the site's infinite-scroll
	// list answers ?page=N with the first N pages' rows, 25 a page, and stops growing at page 40
	// (1,000 rows, ~4.4 MB; page 41 and page 600 are byte-identical to it). Sorted by relevance
	// it is the newest 1,000 postings. It is the only way to the postings the sitemap's
	// 10,000-entry cap leaves out.
	clearedjobsNewestURL = clearedjobsBaseURL + "/jobs?page=40"

	// clearedjobsRequestInterval paces every request of a run at one a second — a courtesy
	// ceiling, not a measured limit: robots.txt names no crawl delay, and a posting page took
	// ~1.9 s through the Scrapling tier from a residential IP.
	clearedjobsRequestInterval = time.Second
	clearedjobsDetailWorkers   = 3
	// clearedjobsRetries bounds how often a refused (403/429) or failed (5xx) request is asked
	// again.
	clearedjobsRetries      = 3
	clearedjobsRetryBackoff = 3 * time.Second

	// clearedjobsFreshPerRun and clearedjobsBacklogPerRun split one run's 1,200 posting pages
	// (~215 KB each, ~20 minutes at one a second) into hydrationLanes' two lanes. The listing
	// names neither employer nor title, so nothing can be judged before a page is bought, and a
	// posting the pipeline discards unsaved (an employer already covered first-party — most of
	// this board's employers run an ATS) stays unseen and would be bought again on every run;
	// newest-first alone would let those fill the budget for good. A cold catalogue is ~11k
	// postings, drained in about ten runs.
	clearedjobsFreshPerRun    = 900
	clearedjobsBacklogPerRun  = 300
	clearedjobsDeadlineMargin = 5 * time.Minute

	// clearedjobsSweepGrace is how long a posting may go unlisted before the sweep closes it.
	// The listing is not the whole board (see the type doc): a posting newer than the sitemap
	// that scrolls out of the newest 1,000 is live yet unlisted, so it reads as unseen on the
	// 48-hour default and would be closed and reopened. Postings run 90 days (validThrough), and
	// a taken-down one answers 410, which cmd/liveness's probe can read.
	clearedjobsSweepGrace = 14 * 24 * time.Hour
)

// NewClearedJobs builds the adapter over the given client.
func NewClearedJobs(c clearedjobsHTTP) Source {
	return clearedjobs{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(clearedjobsRequestInterval), 1),
		freshPerRun:    clearedjobsFreshPerRun,
		backlogPerRun:  clearedjobsBacklogPerRun,
		deadlineMargin: clearedjobsDeadlineMargin,
		retryBackoff:   clearedjobsRetryBackoff,
		now:            time.Now,
	}
}

func (clearedjobs) Provider() string { return "clearedjobs" }

// One national board, no per-tenant id.
func (clearedjobs) boardless() {}

// Hundreds of employers post here, each named per posting.
func (clearedjobs) aggregator() {}

// The crawl lists ~96% of the board, not all of it; see clearedjobsSweepGrace.
func (clearedjobs) sweepGrace() time.Duration { return clearedjobsSweepGrace }

// clearedjobsPosting is one listed posting: its id (the number ending the URL slug) and URL.
type clearedjobsPosting struct {
	id  string
	url string
}

// clearedjobsID matches the numeric id that ends a posting's path, /job/<slug>-<id>.
var clearedjobsID = regexp.MustCompile(`^/job/.+-(\d{4,})$`)

// clearedjobsParsePosting reads a listed URL, or ok=false when it is not a posting on the
// site's own host. The trailing number is the identity; the URL is kept as the site spelled it.
func clearedjobsParsePosting(raw string) (clearedjobsPosting, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != clearedjobsHost {
		return clearedjobsPosting{}, false
	}
	m := clearedjobsID.FindStringSubmatch(u.Path)
	if m == nil {
		return clearedjobsPosting{}, false
	}
	return clearedjobsPosting{id: m[1], url: clearedjobsBaseURL + u.Path}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s clearedjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemap and the newest-postings page name, refreshes the ones
// already stored, and reads the pages hydrationLanes picks out of the rest. A posting whose page
// cannot be read is not yielded; a later run tries again.
func (s clearedjobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []clearedjobsPosting
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.freshPerRun, s.backlogPerRun, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, clearedjobsDetailWorkers, func(p clearedjobsPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("clearedjobs: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone (410); postings left for want of time are not failures.
	if int64(len(picked)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("clearedjobs: requested %d posting pages and none answered", int64(len(picked))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("clearedjobs: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred := len(unseen) - len(picked); deferred > 0 {
		log.Printf("clearedjobs: %d new postings past this run's bound of %d, deferred to later runs", deferred, len(picked))
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s clearedjobs) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list returns every posting the sitemap and the newest-postings page name, newest (highest id)
// first. The sitemap is the base: failing to read it fails the crawl. The newest-postings page
// is best-effort — it adds the postings the sitemap's cap leaves out, and a run without it is
// only stale, never wrong.
func (s clearedjobs) list(ctx context.Context) ([]clearedjobsPosting, error) {
	var index sitemapDoc
	if err := s.retry(ctx, func() (err error) {
		index, err = getSitemap(ctx, s.http, clearedjobsSitemapURL)
		return err
	}); err != nil {
		return nil, fmt.Errorf("clearedjobs: sitemap index: %w", err)
	}
	ids := make(map[string]bool)
	var out []clearedjobsPosting
	add := func(raw string) {
		if p, ok := clearedjobsParsePosting(raw); ok && !ids[p.id] {
			ids[p.id] = true
			out = append(out, p)
		}
	}
	shards := 0
	for _, sm := range index.Sitemaps {
		if !strings.Contains(sm.Loc, clearedjobsShardMarker) {
			continue
		}
		shards++
		var doc sitemapDoc
		if err := s.retry(ctx, func() (err error) {
			doc, err = getSitemap(ctx, s.http, sm.Loc)
			return err
		}); err != nil {
			return nil, fmt.Errorf("clearedjobs: shard %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			add(u.Loc)
		}
	}
	if shards == 0 || len(out) == 0 {
		return nil, fmt.Errorf("clearedjobs: sitemap names no posting (%d job shards in the index)", shards)
	}
	fromSitemap := len(out)
	var newest *html.Node
	if err := s.retry(ctx, func() (err error) {
		newest, err = s.http.GetHTML(ctx, clearedjobsNewestURL)
		return err
	}); err != nil {
		log.Printf("clearedjobs: newest-postings page unreadable; listing the sitemap's %d only: %v", fromSitemap, err)
	} else {
		base, _ := url.Parse(clearedjobsBaseURL)
		for _, link := range jobLinks(base, newest, func(href string) bool { return strings.Contains(href, "/job/") }) {
			add(link)
		}
		log.Printf("clearedjobs: %d postings from the sitemap, %d more from the newest-postings page", fromSitemap, len(out)-fromSitemap)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].id)
		b, _ := strconv.Atoi(out[j].id)
		return a > b
	})
	return out, nil
}

// retry paces and runs call, asking again after a refusal (403/429) or a server failure, which
// the Scrapling client does not retry itself.
func (s clearedjobs) retry(ctx context.Context, call func() error) error {
	return retryWhile(ctx, clearedjobsRetries, s.retryBackoff, clearedjobsRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		return call()
	})
}

func clearedjobsRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// clearedjobsLD is the part of the posting page's schema.org JobPosting the adapter reads.
// Industry carries the posting's clearance level, not an industry.
type clearedjobsLD struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Industry       string `json:"industry"`
	EmploymentType string `json:"employmentType"`
	DatePosted     string `json:"datePosted"`
	JobLocation    struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// errClearedjobsNoPosting marks a page that answered without a JobPosting. It is not the site's
// answer that the posting is gone — a taken-down posting answers 410 — and a challenge page
// answers without one too, so it counts as no answer.
var errClearedjobsNoPosting = errors.New("page carries no JobPosting")

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410) or carries no usable posting; an error is the absence of an answer.
func (s clearedjobs) detail(ctx context.Context, p clearedjobsPosting) (Job, bool, error) {
	var ld clearedjobsLD
	found := false
	err := s.retry(ctx, func() error {
		root, err := s.http.GetHTML(ctx, p.url)
		if err != nil {
			return err
		}
		found = ldJobPosting(root, &ld)
		return nil
	})
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if !found {
		return Job{}, false, errClearedjobsNoPosting
	}
	job, ok := clearedjobsJob(ld, p)
	return job, ok, nil
}

// clearedjobsJob maps a posting's JobPosting to a Job. ok is false for one with no title,
// employer or body.
func clearedjobsJob(ld clearedjobsLD, p clearedjobsPosting) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.Join(strings.Fields(ld.HiringOrganization.Name), " ")
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	// The level is stated in a structured field and nowhere in the text; a labelled line is the
	// form the clearance dictionary reads as an assertion.
	if level := strings.Join(strings.Fields(ld.Industry), " "); level != "" {
		body += "\n<p>Security Clearance: " + level + "</p>"
	}
	addr := ld.JobLocation.Address
	j := Job{
		ExternalID:  p.id,
		URL:         p.url,
		Title:       title,
		Company:     company,
		Description: body,
		Location: joinNonEmpty(
			strings.TrimSpace(addr.Locality),
			strings.TrimSpace(addr.Region),
			strings.TrimSpace(addr.Country),
		),
		EmploymentType: clearedjobsEmploymentType(ld.EmploymentType),
		PostedAt:       NotFuture(parseRFC3339(strings.TrimSpace(ld.DatePosted))),
	}
	if code := location.NormalizeCountry(strings.TrimSpace(addr.Country)); code != "" {
		j.Countries = []string{code}
	}
	return j, true
}

// clearedjobsEmploymentType maps the JobPosting's employment type; "OTHER", which is half the
// board, says nothing.
func clearedjobsEmploymentType(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "CONTRACTOR", "TEMPORARY":
		return "contract"
	case "INTERN":
		return "internship"
	}
	return ""
}
