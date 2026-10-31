package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"
	"time"
)

// gulftalent adapts GulfTalent.com, a major Gulf job board. It is a boardless multi-company
// aggregator: one crawl enumerates the site's sitemap index, follows the job-posting shards
// (sitemap_jx0NN.xml — the jl/jc/co shards are category/company/course pages, not postings),
// fetches each job-detail page, and reads its self-contained schema.org JobPosting — so the
// employer comes from the posting (hiringOrganization), not a configured entry. GulfTalent's
// Akamai edge 403s Go's default TLS+HTTP/2 fingerprint, so in production the adapter is wired with
// the shared Chrome-fingerprint transport (fingerprintHTTP) rather than the shared client. Keyless.
//
// That fingerprint stopped passing too; production crawls it through the Scrapling tier's
// browser (scraplingtier.go), ~2 s a detail page at four tabs. The sitemap lists ~36k postings,
// so the whole sitemap is read every run (the sweep relies on it) but only NEW postings are
// hydrated, a bounded number a run — re-reading every posting each run took a run past four
// hours.
//
// The listing is the whole catalogue or an error, so gulftalent is swept by source
// (fullCatalog): a posting gone from the sitemap closes whatever company it belonged to.

// gulftalentHTTP is the transport gulftalent needs: XML sitemaps plus HTML detail pages.
type gulftalentHTTP interface {
	XMLGetter
	HTMLGetter
}

type gulftalent struct {
	http gulftalentHTTP
	// newestPerRun bounds how many of the newest new postings (the sitemap's head) one run
	// hydrates; backlogPerRun how many more it takes from the rest, through a window that moves
	// every day (see hydrationQueue). Everything else is deferred — not yielded, so not stored.
	newestPerRun, backlogPerRun int
	// now dates the backlog window; nil is time.Now.
	now func() time.Time
}

// The hydration budget. A run of 500 detail pages took 15 minutes on 2026-10-30 with no page
// refused, so 1500 is about 45 minutes against the provider's four-hour timeout. The newest 500
// keep fresh postings arriving within a day; the backlog window of 1000 walks the ~33k older new
// postings in about a month.
const (
	gulftalentNewestPerRun  = 500
	gulftalentBacklogPerRun = 1000
)

// NewGulfTalent builds the GulfTalent adapter over the given HTTP client (the shared
// Chrome-fingerprint fingerprintHTTP in production).
func NewGulfTalent(c gulftalentHTTP) Source {
	return gulftalent{http: c, newestPerRun: gulftalentNewestPerRun, backlogPerRun: gulftalentBacklogPerRun}
}

func (gulftalent) Provider() string { return "gulftalent" }

// boardless marks gulftalent as having no per-tenant board id: one crawl covers the whole site.
func (gulftalent) boardless() {}

// aggregator keeps gulftalent in the source facet: one crawl aggregates postings from many
// companies, so filtering by source=gulftalent is meaningful (not redundant with the company
// filter).
func (gulftalent) aggregator() {}

// fullCatalog: listing reads the sitemap index and every job shard, and fails on any of them
// unreadable or on a listing with no posting in it, so a finished crawl has seen every posting
// the site lists. See the fullCatalog interface (source.go) for the bar.
func (gulftalent) fullCatalog() {}

const (
	gulftalentSitemapURL = "https://www.gulftalent.com/sitemap.xml"
	// gulftalentJobShardMarker selects the job-posting sitemap shards (sitemap_jx0NN.xml) from the
	// index; the jl/jc/co shards are category, company, and course pages that carry no JobPosting.
	gulftalentJobShardMarker = "_jx"
	// gulftalentDetailWorkers bounds the detail fan-out below the shared defaultDetailWorkers (8):
	// GulfTalent's Akamai edge throttles fast bursts and the fingerprint transport does not retry,
	// so a wide fan-out across a large catalogue would silently drop postings.
	gulftalentDetailWorkers = 4
)

// gulftalentJobIDPattern captures the numeric id at the end of a GulfTalent job-detail path
// (/<country>/jobs/<slug>-<id>). It requires the /jobs/ segment so a /companies/ or
// /jobs/category/ URL is not mistaken for a posting, and anchors the id to the end.
var gulftalentJobIDPattern = regexp.MustCompile(`/jobs/[^/]+-(\d+)/?$`)

// gulftalentJobID extracts the native GulfTalent posting id from a job-detail URL, "" when the URL
// is not a job-detail page or carries no trailing id. Any query string or fragment is stripped
// first so a URL with a suffix still matches.
func gulftalentJobID(loc string) string {
	return firstSubmatch(gulftalentJobIDPattern, trimURLSuffix(loc))
}

// gulftalentLDPosting is the slice of the page's schema.org JobPosting the adapter reads.
// jobLocation is a single Place whose address carries the country the geography dictionary reads.
type gulftalentLDPosting struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	DatePosted  string          `json:"datePosted"`
	HiringOrg   gulftalentOrg   `json:"hiringOrganization"`
	JobLocation gulftalentPlace `json:"jobLocation"`
}

type gulftalentOrg struct {
	Name string `json:"name"`
}

type gulftalentPlace struct {
	Address gulftalentAddress `json:"address"`
}

type gulftalentAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressCountry  string `json:"addressCountry"`
}

// Fetch hydrates every listed posting: the path for callers that supply no seen set. FetchNew
// is the one ingest uses.
func (g gulftalent) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	links, err := g.listing(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(links, gulftalentDetailWorkers, func(link string) (Job, bool) {
		return g.detail(ctx, link)
	}), nil
}

// FetchNew reads the whole sitemap and hydrates only what the catalogue lacks: a seen posting
// is a liveness refresh with no request of its own, and new ones are hydrated up to the per-run
// budget (see hydrationQueue). A new posting past the budget is deferred rather than stored
// half-read.
func (g gulftalent) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	links, err := g.listing(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []string
	for _, link := range links {
		if id := gulftalentJobID(link); seen(id) {
			jobs = append(jobs, Job{ExternalID: id, URL: link, SeenRefresh: true})
		} else {
			fresh = append(fresh, link)
		}
	}
	queue := g.hydrationQueue(fresh)
	if deferred := len(fresh) - len(queue); deferred > 0 {
		log.Printf("gulftalent: hydrating %d of %d new postings; %d deferred to a later run", len(queue), len(fresh), deferred)
	}
	return append(jobs, fetchDetails(queue, gulftalentDetailWorkers, func(link string) (Job, bool) {
		return g.detail(ctx, link)
	})...), nil
}

// hydrationQueue picks the new postings this run hydrates: the newest newestPerRun (the sitemap
// lists newest first), then backlogPerRun more from the rest, starting at an offset that
// advances by the window's width every day and wraps — so successive daily runs walk the whole
// backlog. A fixed window starved it for good: a posting whose company an ATS already covers is
// skipped after hydration, never stored, and so stays new forever — measured 2026-10-30, 383 of
// each run's 500 hydrations were such postings, the same ones every day, while ~33.5k live
// postings further down the sitemap were never read.
func (g gulftalent) hydrationQueue(fresh []string) []string {
	head := min(g.newestPerRun, len(fresh))
	queue := slices.Clone(fresh[:head])
	rest := fresh[head:]
	width := min(g.backlogPerRun, len(rest))
	if width <= 0 {
		return queue
	}
	now := time.Now
	if g.now != nil {
		now = g.now
	}
	day := int(now().Unix() / 86400)
	start := (day % len(rest)) * width % len(rest)
	for i := range width {
		queue = append(queue, rest[(start+i)%len(rest)])
	}
	return queue
}

// listing reads the sitemap index and every job-posting shard and returns the posting links. A
// shard that cannot be read, or a listing with no posting at all, fails the whole listing:
// gulftalent is swept by source, so a listing missing a shard would close every posting in it.
func (g gulftalent) listing(ctx context.Context) ([]string, error) {
	index, err := getSitemap(ctx, g.http, gulftalentSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("gulftalent: sitemap index: %w", err)
	}
	var links []string
	for _, s := range index.Sitemaps {
		if !strings.Contains(s.Loc, gulftalentJobShardMarker) {
			continue // only the job-posting shards carry JobPostings
		}
		shard, err := getSitemap(ctx, g.http, s.Loc)
		if err != nil {
			return nil, fmt.Errorf("gulftalent: shard %s: %w", s.Loc, err)
		}
		for _, u := range shard.URLs {
			if gulftalentJobID(u.Loc) != "" {
				links = append(links, u.Loc)
			}
		}
	}
	// A sitemap with no posting in it is not an empty site but a page other than the one we
	// read (a challenge, a moved layout), and the source-scoped sweep would read it as every
	// posting gone.
	if len(links) == 0 {
		return nil, fmt.Errorf("gulftalent: sitemap listed no job posting (%d shards in the index)", len(index.Sitemaps))
	}
	return links, nil
}

// detail fetches one job page and maps its ld+json JobPosting to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, has no resolvable employer (company-less), or has
// no parseable id — so the caller skips just that posting.
func (g gulftalent) detail(ctx context.Context, link string) (Job, bool) {
	id := gulftalentJobID(link)
	if id == "" {
		return Job{}, false
	}
	root, err := g.http.GetHTML(ctx, link)
	if err != nil {
		return Job{}, false
	}
	var p gulftalentLDPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	company := strings.TrimSpace(p.HiringOrg.Name)
	if company == "" {
		return Job{}, false
	}

	location := joinNonEmpty(
		strings.TrimSpace(p.JobLocation.Address.AddressLocality),
		strings.TrimSpace(p.JobLocation.Address.AddressCountry),
	)
	return Job{
		ExternalID:  id,
		URL:         link,
		Title:       strings.TrimSpace(p.Title),
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}
