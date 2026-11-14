package sources

import (
	"context"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// concurrencyLimitedJSONGetter bounds how many GetJSON calls are in flight at once via a shared
// semaphore, independent of the pipeline's board-worker pool. Unlike a rate limiter — which caps
// the request START rate but lets slow requests pile up concurrently — this caps simultaneous
// in-flight requests, the right lever for an API that degrades under sustained concurrent load
// rather than by rate. One instance carries one semaphore, shared across every board and page.
type concurrencyLimitedJSONGetter struct {
	inner JSONGetter
	sem   chan struct{}
}

// GetJSON acquires a semaphore slot before delegating (releasing it after), so at most cap
// requests run at once; a cancelled context surfaces while waiting and skips the inner fetch.
func (g concurrencyLimitedJSONGetter) GetJSON(ctx context.Context, url string, v any) error {
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-g.sem }()
	return g.inner.GetJSON(ctx, url, v)
}

// concurrencyLimitedJSONPoster is the JSON-POST counterpart of concurrencyLimitedJSONGetter: a
// shared semaphore bounds how many PostJSON calls are in flight at once, across every board and
// page of a run.
type concurrencyLimitedJSONPoster struct {
	inner JSONPoster
	sem   chan struct{}
}

// PostJSON acquires a semaphore slot before delegating (releasing it after); a cancelled context
// surfaces while waiting and skips the inner request.
func (p concurrencyLimitedJSONPoster) PostJSON(ctx context.Context, url string, body, v any) error {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.sem }()
	return p.inner.PostJSON(ctx, url, body, v)
}

// limitedOorwinPoster wraps a poster with a fresh semaphore shared across one registry build:
// every Oorwin tenant is answered by the same API host, and a detail takes seconds to build.
func limitedOorwinPoster(c JSONPoster) JSONPoster {
	return concurrencyLimitedJSONPoster{inner: c, sem: make(chan struct{}, oorwinMaxInFlight)}
}

// concurrencyLimitedHTMLGetter is the HTML-GET counterpart of concurrencyLimitedJSONGetter: it
// bounds how many GetHTML calls are in flight at once via a shared semaphore, independent of
// the pipeline's board-worker pool — the right lever for a host that degrades under sustained
// concurrent load rather than by rate. One instance carries one semaphore, shared across every
// board and page.
type concurrencyLimitedHTMLGetter struct {
	inner HTMLGetter
	sem   chan struct{}
}

// GetHTML acquires a semaphore slot before delegating (releasing it after), so at most cap
// requests run at once; a cancelled context surfaces while waiting and skips the inner fetch.
func (g concurrencyLimitedHTMLGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-g.sem }()
	return g.inner.GetHTML(ctx, url)
}

// opendata.trudvsem.ru answers a page in ~0.5s in isolation and tolerates a brief burst, but its
// gov infra degrades under the SUSTAINED concurrent load of the pipeline's 8 board workers
// hammering it for a whole crawl — intermittent 500s and slow bodies that trip the 15s read
// timeout, failing most regions (a rate limiter did not help, since slow reads keep the workers
// busy and never wait on it). Bounding in-flight requests to a gentle few keeps the crawl in the
// API's healthy regime.
//
// The cap was 2 when a page took ~0.5s. Measured 2026-10-30 from the crawl host a page takes
// 5.5-6.5s, nearly all of it server time before the first byte, so 2 in flight read ~0.36
// pages/s: a 40-minute crawl reached ~11 of the 90 regions, the eight boards running at once
// split the two slots so a 100-page region needed most of a run, and the big regions were the
// ones still in flight at the deadline every time — Bashkortostan went unseen for a week. Six
// requests held in flight for a sustained minute answered all 200 at 6.7s on average, so the
// latency is per request rather than load, and 6 roughly triples the rate (~0.9 pages/s, the ~5,100-page
// catalogue in two to three runs) without nearing the earlier failure regime.
const trudvsemMaxInFlight = 6

// limitedTrudvsemGetter wraps a getter with a fresh semaphore shared across one registry build, so
// all of trudvsem's region-shard requests in a run stay under one gentle in-flight cap.
func limitedTrudvsemGetter(c JSONGetter) JSONGetter {
	return concurrencyLimitedJSONGetter{inner: c, sem: make(chan struct{}, trudvsemMaxInFlight)}
}

// portal-api.emagine.org serves a single detail request in ~0.9s and tolerates a burst (80
// back-to-back at 8-way all returned 200), but a whole-catalogue hydration — 1025 details in one
// run — starts refusing CONNECTIONS near the end: `dial tcp: i/o timeout`, already past the
// client's own three attempts, on ~1% of postings clustered in the run's tail. So the trigger is
// sustained concurrency over a long run, not rate, and the loss outlives the run: a posting
// ingested list-only is re-offered for hydration only while it is younger than
// pipeline.HydrationRetryWindow (14 days), and is seen like any other row after that. Four in
// flight halves the pressure while keeping a full first crawl inside the ingest window; later
// crawls hydrate only new postings, so the cap costs nothing in steady state. Tune from the
// observed loss rate.
const emagineMaxInFlight = 4

// limitedEmagineGetter wraps a getter with a fresh semaphore shared across one registry build, so
// the whole detail fan-out of a run competes for the same few in-flight slots.
func limitedEmagineGetter(c JSONGetter) JSONGetter {
	return concurrencyLimitedJSONGetter{inner: c, sem: make(chan struct{}, emagineMaxInFlight)}
}

// The WhatJobs feed serves sequential requests happily — a dozen back-to-back from one IP never
// tripped anything — but the pipeline crawls board files in parallel, and on the provider's first
// production run 8 of its 10 keyword boards failed with HTTP 429. So the trigger is simultaneity, not
// rate. Two in flight is gentle enough to clear it, and cheap now that a keyword crawl ends when its
// relevance collapses (typically 2 pages, not 40) rather than walking the page budget.
const whatjobsMaxInFlight = 2

// limitedWhatJobsGetter wraps a getter with a fresh semaphore shared across one registry build, so
// every keyword board in a run competes for the same small number of in-flight requests.
func limitedWhatJobsGetter(c JSONGetter) JSONGetter {
	return concurrencyLimitedJSONGetter{inner: c, sem: make(chan struct{}, whatjobsMaxInFlight)}
}

// Wellfound's requests go through the Scrapling tier over the proxy — one shared exit IP that
// the other proxied providers use too, against an edge whose own limit nobody has measured.
//
// The cap dates from the hosted Firecrawl tier, which carried wellfound until 2026-10, and the
// incident that set it is the reason it stays until a run through the new tier says otherwise.
// Found live 2026-09-14, twice. First: crawling wellfound's 11 role-slice boards in one ingest
// run unbounded, 4 boards failed with "vendor rate limit did not lift after 4 attempts" within
// three minutes. The first fix paced the request START rate (one every 2s, rateLimitedHTMLGetter)
// — but a re-run under that fix still failed 10 of 11 boards the same way within ~3 minutes,
// because Firecrawl's own request (bypassing Wellfound's Cloudflare challenge) takes several
// seconds to complete, so the next paced board's request starts and overlaps with it — several
// requests end up in flight simultaneously despite the spaced-out starts. The real ceiling is on
// CONCURRENT requests, not request rate: the same "simultaneity, not rate" shape
// whatjobs/trudvsem/emagine already document above. Each failure discarded an otherwise-
// successful partial crawl, per this adapter's own "a later page failure discards the whole
// board" rule — a real, not hypothetical, cost, twice over.
//
// One in flight is deliberately the most conservative cap available, since the true ceiling is
// unknown and the exit IP is shared with providers this file cannot coordinate with; tune upward
// only from observed convergence, the same discipline every other cap in this file follows.
const wellfoundMaxInFlight = 1

// limitedWellfoundGetter wraps a getter with a fresh semaphore shared across one registry
// build, so every board's paginated fetch in a run competes for the same single in-flight slot.
func limitedWellfoundGetter(c HTMLGetter) HTMLGetter {
	return concurrencyLimitedHTMLGetter{inner: c, sem: make(chan struct{}, wellfoundMaxInFlight)}
}

// Potok served a 44k-candidate tenant probe at ~12 req/s from 3 workers without one refusal
// (2026-10-14), and nothing faster was measured. Unbounded, a run's board workers each fan out
// defaultDetailWorkers posting-page requests, so the in-flight count is held to that measured 3.
const potokMaxInFlight = 3

// limitedPotokGetter wraps a getter with a fresh semaphore shared across one registry build, so
// every Potok board's listing and posting pages in a run compete for the same few slots.
func limitedPotokGetter(c HTMLGetter) HTMLGetter {
	return concurrencyLimitedHTMLGetter{inner: c, sem: make(chan struct{}, potokMaxInFlight)}
}

// coveto serves every tenant from one platform — 850 live tenants and 66,443 open postings
// measured 2026-10-25, a white-labelled tenant's pages included — and each posting's body is its
// own page, so a first crawl is tens of thousands of page requests to one origin. 80 posting
// pages at 8 in flight all answered 200 (~0.9s each); nothing wider was measured, and the
// pipeline's board workers would each fan out defaultDetailWorkers on top of one another. So the
// whole provider's posting pages share the measured 8 — Teamtailor's bulk 403 is the shape of
// what an unbounded per-posting fan-out earns. The feed requests, one per board, are not capped.
const covetoMaxInFlight = 8

// limitedCoveto routes coveto's posting-page requests through one shared semaphore and passes
// its feed requests through untouched.
type limitedCoveto struct {
	covetoGetter
	pages HTMLGetter
}

func (g limitedCoveto) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	return g.pages.GetHTML(ctx, url)
}

// limitedCovetoGetter wraps a getter with a fresh semaphore shared across one registry build.
func limitedCovetoGetter(c covetoGetter) covetoGetter {
	return limitedCoveto{covetoGetter: c, pages: concurrencyLimitedHTMLGetter{inner: c, sem: make(chan struct{}, covetoMaxInFlight)}}
}

// helixjobs.com serves every iid's list and posting pages from one origin — 590 iids and 25,021
// postings measured 2026-10-25 — so a first crawl is tens of thousands of page requests to it.
// A walk of all 1,001 portal list pages and 120 posting pages at 4 in flight all answered (one
// connect timeout, gone on retry); nothing wider was measured. So the whole provider shares the
// measured 4, its list pages included: unlike coveto's single feed, a large iid's list is itself
// a hundred-page walk.
const helixjobsMaxInFlight = 4

// limitedHelixjobs routes every helixjobs request through one shared semaphore.
type limitedHelixjobs struct {
	inner HTMLResolvedGetter
	sem   chan struct{}
}

func (g limitedHelixjobs) GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error) {
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	defer func() { <-g.sem }()
	return g.inner.GetHTMLResolved(ctx, url)
}

// limitedHelixjobsGetter wraps a getter with a fresh semaphore shared across one registry build.
func limitedHelixjobsGetter(c HTMLResolvedGetter) HTMLResolvedGetter {
	return limitedHelixjobs{inner: c, sem: make(chan struct{}, helixjobsMaxInFlight)}
}

// FriendWork's API meters simultaneity AND rate: two parallel workers drew 429s, while sequential
// requests at ~2/s were clean (2026-10-14). So its requests run one at a time AND no faster than
// that measured pace — a single listing can be ~2 MB, and a rate limit alone would let the next
// request start while it is still downloading.
const (
	friendworkMaxInFlight     = 1
	friendworkRequestInterval = 500 * time.Millisecond // ~2 req/s
	friendworkRequestBurst    = 1
)

// limitedFriendWorkGetter wraps a getter with one semaphore and one limiter shared across one
// registry build, so every FriendWork board in a run is fetched sequentially at the measured pace.
func limitedFriendWorkGetter(c JSONGetter) JSONGetter {
	return concurrencyLimitedJSONGetter{
		inner: rateLimitedJSONGetter{
			inner:   c,
			limiter: rate.NewLimiter(rate.Every(friendworkRequestInterval), friendworkRequestBurst),
		},
		sem: make(chan struct{}, friendworkMaxInFlight),
	}
}

// HireClick serves every tenant from one origin, and its pages are 1-3 MB delivered at
// ~80-160 KB/s per connection, so concurrent downloads split one small pipe rather than add to
// it. On 2026-10-20 a test crawl of five boards at the default fan-out (eight posting pages at
// once) timed out on most of one board's pages, and within minutes the origin was taking 35-90s
// to serve its 442-byte robots.txt and refusing connections from a second network as well. Two
// in flight across the whole run — board pages, job lists and posting pages alike — is the
// conservative cap; tune upward only from observed convergence.
const hireclickMaxInFlight = 2

// limitedHireClickGetter bounds every HireClick request in a run under one shared semaphore.
type limitedHireClickGetter struct {
	inner hireclickHTTP
	sem   chan struct{}
}

// limitedHireClick wraps a client with a fresh semaphore shared across one registry build.
func limitedHireClick(c hireclickHTTP) hireclickHTTP {
	return limitedHireClickGetter{inner: c, sem: make(chan struct{}, hireclickMaxInFlight)}
}

// acquire takes a slot, or reports the context's error when it is cancelled while waiting.
func (g limitedHireClickGetter) acquire(ctx context.Context) error {
	select {
	case g.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g limitedHireClickGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := g.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-g.sem }()
	return g.inner.GetHTML(ctx, url)
}

func (g limitedHireClickGetter) GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error) {
	if err := g.acquire(ctx); err != nil {
		return nil, "", err
	}
	defer func() { <-g.sem }()
	return g.inner.GetHTMLResolved(ctx, url)
}

func (g limitedHireClickGetter) GetJSON(ctx context.Context, url string, v any) error {
	if err := g.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-g.sem }()
	return g.inner.GetJSON(ctx, url, v)
}
