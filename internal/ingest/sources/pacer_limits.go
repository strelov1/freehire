package sources

import (
	"context"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// concurrencyLimitedJSONGetter bounds in-flight GetJSON calls with a shared semaphore,
// independent of the board-worker pool. Unlike a rate limiter it caps simultaneous requests, the
// right lever for an API that degrades under sustained concurrency. One instance, one semaphore,
// shared across every board and page.
type concurrencyLimitedJSONGetter struct {
	inner JSONGetter
	sem   chan struct{}
}

// GetJSON holds a semaphore slot while delegating; a cancelled context surfaces while waiting.
func (g concurrencyLimitedJSONGetter) GetJSON(ctx context.Context, url string, v any) error {
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-g.sem }()
	return g.inner.GetJSON(ctx, url, v)
}

// concurrencyLimitedJSONPoster is the JSON-POST counterpart of concurrencyLimitedJSONGetter.
type concurrencyLimitedJSONPoster struct {
	inner JSONPoster
	sem   chan struct{}
}

// PostJSON holds a semaphore slot while delegating; a cancelled context surfaces while waiting.
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

// concurrencyLimitedHTMLGetter is the HTML-GET counterpart of concurrencyLimitedJSONGetter.
type concurrencyLimitedHTMLGetter struct {
	inner HTMLGetter
	sem   chan struct{}
}

// GetHTML holds a semaphore slot while delegating; a cancelled context surfaces while waiting.
func (g concurrencyLimitedHTMLGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-g.sem }()
	return g.inner.GetHTML(ctx, url)
}

// opendata.trudvsem.ru tolerates a brief burst, but its gov infra degrades under SUSTAINED
// concurrent load from the board workers: intermittent 500s and slow bodies that trip the read
// timeout. A rate limiter did not help, since slow reads keep the workers busy and never wait on
// it. Bounding in-flight requests keeps the crawl in the API's healthy regime. Pages are slow
// (seconds to first byte) and that latency is per request, not load, so the cap is a few in
// flight, enough that big regions finish before the run deadline.
const trudvsemMaxInFlight = 6

// limitedTrudvsemGetter wraps a getter with a fresh semaphore shared across one registry build, so
// all of trudvsem's region-shard requests in a run stay under one gentle in-flight cap.
func limitedTrudvsemGetter(c JSONGetter) JSONGetter {
	return concurrencyLimitedJSONGetter{inner: c, sem: make(chan struct{}, trudvsemMaxInFlight)}
}

// portal-api.emagine.org tolerates a burst, but a whole-catalogue hydration starts refusing
// CONNECTIONS (`dial tcp: i/o timeout`) near the end of the run. The trigger is sustained
// concurrency over a long run, not rate, and the loss outlives the run: a posting ingested
// list-only is re-offered for hydration only while younger than pipeline.HydrationRetryWindow.
// The cap keeps a full first crawl inside the ingest window; later crawls hydrate only new
// postings. Tune from the observed loss rate.
const emagineMaxInFlight = 4

// limitedEmagineGetter wraps a getter with a fresh semaphore shared across one registry build, so
// the whole detail fan-out of a run competes for the same few in-flight slots.
func limitedEmagineGetter(c JSONGetter) JSONGetter {
	return concurrencyLimitedJSONGetter{inner: c, sem: make(chan struct{}, emagineMaxInFlight)}
}

// The WhatJobs feed serves sequential requests happily, but parallel keyword boards draw HTTP
// 429. The trigger is simultaneity, not rate. Two in flight clears it, and is cheap since a
// keyword crawl ends when its relevance collapses rather than walking the page budget.
const whatjobsMaxInFlight = 2

// limitedWhatJobsGetter wraps a getter with a fresh semaphore shared across one registry build, so
// every keyword board in a run competes for the same small number of in-flight requests.
func limitedWhatJobsGetter(c JSONGetter) JSONGetter {
	return concurrencyLimitedJSONGetter{inner: c, sem: make(chan struct{}, whatjobsMaxInFlight)}
}

// Wellfound's requests go through the Scrapling tier over the proxy: one exit IP shared with
// other proxied providers, against an edge whose limit is unknown. Pacing the request START
// rate failed because a challenge-bypassing request takes seconds and overlaps the next, so the
// ceiling is on CONCURRENT requests. A later page failure discards the whole board, so a trip
// is costly. One in flight is the most conservative cap; tune upward only from observed
// convergence.
const wellfoundMaxInFlight = 1

// limitedWellfoundGetter wraps a getter with a fresh semaphore shared across one registry
// build, so every board's paginated fetch in a run competes for the same single in-flight slot.
func limitedWellfoundGetter(c HTMLGetter) HTMLGetter {
	return concurrencyLimitedHTMLGetter{inner: c, sem: make(chan struct{}, wellfoundMaxInFlight)}
}

// Potok tolerated 3 concurrent workers without refusal and nothing wider was tried. Unbounded,
// each board worker fans out defaultDetailWorkers posting-page requests, so in-flight is held to 3.
const potokMaxInFlight = 3

// limitedPotokGetter wraps a getter with a fresh semaphore shared across one registry build, so
// every Potok board's listing and posting pages in a run compete for the same few slots.
func limitedPotokGetter(c HTMLGetter) HTMLGetter {
	return concurrencyLimitedHTMLGetter{inner: c, sem: make(chan struct{}, potokMaxInFlight)}
}

// coveto serves every tenant from one platform and each posting's body is its own page, so a
// first crawl is tens of thousands of requests to one origin. 8 in flight was tried clean and
// nothing wider; unbounded, board workers would each fan out defaultDetailWorkers on top of one
// another (Teamtailor's bulk 403 is what that earns). The whole provider's posting pages share
// the cap; the per-board feed requests are not capped.
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

// helixjobs.com serves every iid's list and posting pages from one origin, so a first crawl is
// tens of thousands of requests. 4 in flight was tried clean and nothing wider. The whole
// provider shares the cap, list pages included: unlike coveto's single feed, a large iid's list
// is itself a hundred-page walk.
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

// FriendWork's API meters simultaneity AND rate: two parallel workers drew 429s, sequential
// requests at ~2/s were clean. So requests run one at a time AND no faster than that pace; a
// listing can be ~2 MB, and a rate limit alone would let the next start mid-download.
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

// HireClick serves every tenant from one origin with 1-3 MB pages at ~100 KB/s per connection,
// so concurrent downloads split one small pipe. At the default fan-out the origin timed out and
// then refused connections even from other networks. Two in flight across the whole run (board
// pages, job lists and posting pages) is the conservative cap; tune upward only from observed
// convergence.
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
