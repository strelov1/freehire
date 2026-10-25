package sources

import (
	"context"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// waiter gates a request until the rate limiter admits it. *rate.Limiter satisfies it;
// tests inject a fake to assert the gate fires without timing flake.
type waiter interface {
	Wait(ctx context.Context) error
}

// rateLimitedHTMLGetter wraps an HTMLGetter with a shared limiter so its aggregate GetHTML
// request rate stays under the limit, independent of the caller's worker concurrency. One
// instance carries one limiter, so every request routed through it — across boards and both
// the listing and detail paths — shares the same token bucket.
type rateLimitedHTMLGetter struct {
	inner   HTMLGetter
	limiter waiter
}

// GetHTML blocks on the limiter before delegating, so a cancelled context surfaces as the
// Wait error and the inner fetch is skipped.
func (g rateLimitedHTMLGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return g.inner.GetHTML(ctx, url)
}

// pacedHTMLGetter wraps a getter with a fresh limiter (one token per interval, burst bucket)
// shared across one registry build, so all of a provider's requests in a run are paced under
// its per-IP window budget. The per-provider interval/burst constants below carry the
// rationale for each rate.
func pacedHTMLGetter(c HTMLGetter, interval time.Duration, burst int) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(interval), burst),
	}
}

// rateLimitedTextGetter is the raw-body counterpart of rateLimitedHTMLGetter.
type rateLimitedTextGetter struct {
	inner   TextGetter
	limiter waiter
}

func (g rateLimitedTextGetter) GetText(ctx context.Context, url string) (string, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return "", err
	}
	return g.inner.GetText(ctx, url)
}

// careers-page.com rate-limits by a per-IP request budget per time window, so a full run must
// hold its aggregate request rate under it (proxy egress and a narrow worker pool cap the
// burst, not the total-per-window — see the careerspage-request-pacer change). The interval is
// conservative because the true budget is unknown: under-shooting only lengthens a run, while
// over-shooting re-introduces the 429 starvation. Tune from observed convergence.
const (
	careerspageRequestInterval = 800 * time.Millisecond // ~1.25 req/s
	careerspageRequestBurst    = 2
)

// vagas.com.br rate-limits by a per-IP request budget: a full national-board crawl (three area
// listings paginated + a detail fan-out over hundreds of postings) fired unpaced through the
// single egress proxy IP 429s that IP and then 429s even spaced requests during the penalty
// window. Its detail pool bursts to defaultDetailWorkers, so the pacer — not the pool — must
// hold the aggregate rate under the window. The interval is more conservative than careerspage's
// because vagas 429'd hard and its true budget is unknown; tune from observed convergence.
const (
	vagasRequestInterval = time.Second // ~1 req/s
	vagasRequestBurst    = 1
)

// www.workstream.us is served by AWS API Gateway and meters by request RATE per IP, on every
// path — the listing and the posting pages alike. A SHORT sample reads the knee near 1 req/s
// (measured 2026-09-02 over cold boards: 1.0 req/s refused nothing over 40, ~1.4 req/s refused
// 48%, 2 req/s 71%), and that reading is wrong for a crawl. A 52-minute harvest paced at one
// token a second was refused 524 of 2,138 requests, and what it sustained was ~0.52 ANSWERED
// requests a second — so the budget is a rate the platform holds you to over the whole run,
// not a burst the first refusal announces.
//
// Two things then make the pace worse than the number it names, both learned the same day:
// the penalty outlives the burst that earned it (a run refused at 2 req/s kept being refused
// minutes later at 0.8), and a refused request RETRIES three times beneath this limiter — the
// backoff lives inside the client, so one paced token can become three requests exactly when
// the platform is already refusing. That is a feedback loop, and it is why the pace sits at
// half the observed ceiling rather than at it: the headroom is what the retries spend.
//
// Its detail fan-out bursts to defaultDetailWorkers, so the pacer — not the pool — is what
// holds the aggregate rate. Tune from the observed refusal rate, downward while boards still
// fail with 429.
const (
	workstreamRequestInterval = 2 * time.Second // ~0.5 req/s
	workstreamRequestBurst    = 1
)

// ClinchTalent fronts detail pages with a rate-based AWS-WAF Challenge action: a cold IP is
// served a handful of clean pages (spike observed ~6) before the WAF flips to a 202 challenge
// and holds a long per-IP penalty. clinch fetches one detail page per new posting, so its
// aggregate rate — not the worker pool — must stay under that window. The interval is
// deliberately gentle (well below the observed trip point) because the true budget is unknown
// and the penalty is punishing: under-shooting only lengthens a run, while over-shooting trips
// the WAF and latches clinch back to sitemap-only for the rest of the run. Tune from the
// observed description-fill rate.
const (
	clinchRequestInterval = 1500 * time.Millisecond // ~0.67 req/s
	clinchRequestBurst    = 1
)

// energyjobline.com's own robots.txt declares Crawl-delay: 10 for every user agent, and its
// sitemap enumerates roughly 171,000 postings (measured 2026-10-07) — honouring the declared
// rate literally would take a single crawl on the order of weeks. This is a policy choice, not
// a measured block-avoidance ceiling (contrast clinch's WAF trip point): the actual bug a first
// unpaced run hit was unrelated to rate — see crawlerUserAgent — and no rate-triggered block has
// been observed here yet. The pace is a deliberate compromise, faster than the site asks for
// and slower than an unpaced crawl, chosen because honouring Crawl-delay literally is
// impractical at this catalogue size. Revisit if energyjobline.com starts refusing requests at
// this rate.
const (
	energyjoblineRequestInterval = 1500 * time.Millisecond // ~0.67 req/s
	energyjoblineRequestBurst    = 1
)

// airswift.com 429'd every detail-page request in its first live crawl (an unpaced
// 8-worker burst over ~1,646 postings, measured 2026-10-08) despite showing no sign of a
// rate limit under light manual probing beforehand — the limit only shows up under
// sustained volume. The penalty also outlives the burst that earned it (still 429 on a
// 3s-spaced probe ~2 minutes after the burst) — cleared after roughly 15 minutes idle.
//
// A short sample at 2s/req (5 requests, ~10s) looked clean and shipped first, but a real
// paced crawl at that rate (892 requests, ~61 minutes) still 429'd on every request — the
// same shape workstream's own pacer note describes: a short sample understates a budget
// that is metered over a longer window, not just a per-request minimum gap. A 15-request
// sample at 5s/req (75s sustained) also looked clean, and ALSO still 429'd on every
// request of a real paced crawl (889 requests, ~2.5 hours including retries, measured
// 2026-10-08). Two short samples in a row have now understated the real budget, which
// looks to be metered over a window materially longer than any sample this codebase has
// patience to run by hand. 15s/req is a larger jump than the previous step, chosen to
// get meaningfully outside the window that burned the first two guesses rather than
// edge toward it again — still not a proven ceiling. If a full paced crawl at 15s/req
// still 429s, treat this provider the way Rigzone was treated: move it to the deferred
// group rather than guessing a fourth time.
const (
	airswiftRequestInterval = 15 * time.Second // ~0.067 req/s
	airswiftRequestBurst    = 1
)

// rateLimitedJSONGetter is the JSON-GET counterpart of rateLimitedHTMLGetter: it wraps a
// JSONGetter with a shared limiter so its aggregate GetJSON rate stays under the endpoint's
// budget, independent of the caller's worker concurrency. One instance carries one limiter, so
// every request routed through it — across boards, listing pages and the detail fan-out —
// shares the same token bucket.
//
// Distinct from concurrencyLimitedJSONGetter below: that one caps how many requests are in
// FLIGHT, the right lever for an API that degrades under sustained concurrent load. This one
// caps the request rate, the right lever for an API that meters requests per unit time and is
// indifferent to how many arrive at once.
type rateLimitedJSONGetter struct {
	inner   JSONGetter
	limiter waiter
}

// GetJSON blocks on the limiter before delegating, so a cancelled context surfaces as the Wait
// error and the inner fetch is skipped.
func (g rateLimitedJSONGetter) GetJSON(ctx context.Context, url string, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.inner.GetJSON(ctx, url, v)
}

// rateLimitedHeaderJSONGetter is rateLimitedJSONGetter for an adapter whose requests carry
// headers. Every step of such a crawl goes through this one method — a call that needs no
// headers passes nil — so one limiter is the whole budget.
type rateLimitedHeaderJSONGetter struct {
	inner   HeaderJSONGetter
	limiter waiter
}

// GetJSONWithHeaders blocks on the limiter before delegating, so a cancelled context surfaces as
// the Wait error and the inner fetch is skipped.
func (g rateLimitedHeaderJSONGetter) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.inner.GetJSONWithHeaders(ctx, url, headers, v)
}

// join.com meters by RATE, and the two were easy to confuse: an unpaced crawl fans 8 board
// workers out over its list endpoint and the refusals looked like a concurrency limit, but
// holding the rate steady and varying only the worker count clears it — 4 workers at 2 req/s
// were refused nothing, while 2 workers left to run flat out (5.7 req/s) lost 17%. Measured
// against live boards 2026-08-18:
//
//	1.1 req/s, 700 requests  →   0% refused      3 req/s, 100 requests → 10% refused
//	2   req/s, 240 requests  →   0% refused      5 req/s, 100 requests → 20% refused
//
// So the knee sits between 2 and 3, and the pace takes the clean side of it. Under-shooting
// only lengthens a run; over-shooting re-enters the refusals that had 3436 of 4749 boards
// failing before this existed.
//
// 2 req/s still left a residue in production: refusals were not random but accelerated
// through a run (8 failures at 368 boards, 44 at 1115, 70 at 1187 — see issue #2090), which
// looks like a cumulative budget layered on top of the per-second one and would not show up
// in the short isolated samples above. 1.5 req/s buys back headroom against that budget; see
// issue #2094 for why that pace needed joinShards raised from 4 to 5 rather than just slowing
// the existing 4 down (TestJoinPaceFitsTheRunBudget pins the arithmetic).
const (
	joinRequestInterval = time.Second * 2 / 3 // 1.5 req/s
	joinRequestBurst    = 2
)

// pacedJoinGetter wraps a getter with a fresh limiter shared across one registry build, so every
// board's listing pages and detail fan-out in a run compete for the same token bucket. Both paths
// are wrapped because both hit the same metered host — unlike seek, whose listing was never
// refused.
func pacedJoinGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(joinRequestInterval), joinRequestBurst),
	}
}

// rateLimitedJSONPoster is the JSON-POST counterpart of rateLimitedHTMLGetter: it wraps a
// JSONPoster with a shared limiter so its aggregate PostJSON rate stays under the endpoint's
// budget, independent of the caller's worker concurrency. It exists because a detail endpoint is
// not always a GET — SEEK's is a GraphQL POST.
type rateLimitedJSONPoster struct {
	inner   JSONPoster
	limiter waiter
}

// PostJSON blocks on the limiter before delegating, so a cancelled context surfaces as the Wait
// error and the inner request is skipped.
func (p rateLimitedJSONPoster) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.PostJSON(ctx, url, body, v)
}

// www.seek.com.au/graphql enforces a per-IP request BUDGET rather than degrading under load: an
// unpaced first crawl fired 3,267 detail POSTs in 95 seconds (43 boards, each detail pool bursting
// to defaultDetailWorkers) and was answered 429 on essentially all of them, stranding 87% of that
// crawl's postings without a description — while a single identical request from the same host
// returned 200 after two minutes idle. An immediate refusal that clears on idle is a window, not
// saturation, which is why this is a rate limiter and not the in-flight cap trudvsem and emagine
// use: eight fast requests a second from one worker is still eight a second.
//
// The rate is ~17x below the one that was refused, and conservative for the same reason every other
// constant here is — the true budget is unknown, and under-shooting only lengthens a run while
// over-shooting re-enters the penalty window. It does mean the ~7.8k first backfill no longer fits
// one ingest window and accretes over a few runs, which is safe only because seek defers a posting
// it could not hydrate instead of storing it body-less. Steady state is a few hundred new postings
// a day and finishes in minutes. Tune from the observed description-fill rate.
const (
	seekDetailInterval = 500 * time.Millisecond // ~2 req/s
	seekDetailBurst    = 2
)

// pacedSeekPoster wraps a poster with a fresh limiter shared across one registry build, so every
// board's detail fan-out in a run competes for the same token bucket. Only the detail path is
// wrapped — seek's search listing is ~150 requests a run and has never been refused.
func pacedSeekPoster(c JSONPoster) JSONPoster {
	return rateLimitedJSONPoster{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(seekDetailInterval), seekDetailBurst),
	}
}

// The SEEK proxy lane (see seekDetailNetwork) runs on its OWN bucket, never the direct one: it
// is a different IP, and SEEK's budget is per IP. Its pace is half the direct lane's because the
// proxy is one static address every proxiedProviders entry already egresses through — eightfold
// and vagas pace themselves to ~1 req/s on it — and losing it to a SEEK ban as well would leave
// the network with no lane at all. 1 req/s still spends the whole seekDetailRunBudget inside
// ~25 minutes, half a run's timeout, should the direct lane be refused on its first request.
const (
	seekProxyDetailInterval = time.Second // ~1 req/s
	seekProxyDetailBurst    = 1
)

// pacedSeekProxyPoster wraps the proxied poster with its own fresh limiter.
func pacedSeekProxyPoster(c JSONPoster) JSONPoster {
	return rateLimitedJSONPoster{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(seekProxyDetailInterval), seekProxyDetailBurst),
	}
}

// Eightfold rate-limits an IP to ~290 requests per window (see eightfold.go), and its crawl
// egresses through the single shared proxy IP alongside a dozen other providers
// (proxiedProviders) — so its own concurrency cap (eightfoldDetailWorkers) and retry backoff
// are not enough on their own; the aggregate request START rate needs its own ceiling,
// independent of how many boards run in parallel. September 2026: unpaced, ~44 of ~95 boards
// were failing their LISTING call (both list-API generations 403ing), the shape of a window
// already spent by the run's own volume rather than a hard IP blocklist (a direct request
// elsewhere in the same run succeeds). The interval below is deliberately conservative — as
// cautious as vagas's, the gentlest existing pace on this shared proxy — because the true
// per-window budget eightfold enforces, and how much of it the other proxied providers'
// concurrent traffic already spends, are both unmeasured. Tune from observed convergence
// (the board_health unhealthy count for this provider), downward while boards still fail
// their listing call and upward only while none do.
const (
	eightfoldRequestInterval = time.Second // ~1 req/s
	eightfoldRequestBurst    = 1
)

// pacedEightfoldGetter wraps a getter with a fresh limiter shared across one registry build,
// so every board's listing pages and detail fan-out in a run compete for the same token
// bucket — both paths hit the same rate-limited edge.
func pacedEightfoldGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(eightfoldRequestInterval), eightfoldRequestBurst),
	}
}

// ADP Workforce Now serves ~2,800 boards from the single shared host workforcenow.adp.com, and
// the adapter re-lists and re-fetches EVERY requisition's detail on every hourly run (it has no
// HydratingSource seen-skip yet), so a run's volume is large and grows with the catalogue. Fired
// unpaced at defaultConcurrency (8 boards at once, each with its own detail fan-out), September
// 2026 measured 2,441 of 2,798 boards taking a 429 on their listing call alone — the shape of a
// window spent by the run's own aggregate rate against one shared host, not a hard IP blocklist
// (workforcenow.adp.com serves the same prod IP fine outside the crawl's own burst). The rate
// below is conservative — deliberately below what the run needs to finish covering the whole
// catalogue inside one hourly window, because under-shooting only leaves boards uncrawled this
// hour (they keep their last-known state and are retried next run) while over-shooting
// re-triggers the exact 429 storm this exists to stop; a true FetchNew/seen-skip (like the other
// hydrating adapters) is the real fix for run time; see freehire#status-2026-09-06. Tune from
// observed convergence (the board_health unhealthy count for this provider), downward while
// boards still 429 and upward only while none do.
const (
	adpRequestInterval = 200 * time.Millisecond // ~5 req/s
	adpRequestBurst    = 2
)

// SmartRecruiters meters its public postings API per caller, and all 5,204 boards reach it
// through one host — the same shape as ADP below, and the same outcome unpaced: measured on
// production 2026-09-16, 5,165 of those boards took a 429 within two days while 5,165 also
// succeeded, so the fleet was spending most of its SmartRecruiters budget on retries and
// 2,246 boards sat in a failed state at any moment.
//
// The rate is measured, not guessed. From a clean egress IP the API served 30 of 30 requests
// at 10 req/s with zero 429s (2026-09-16), so the ceiling is at least that; this sits at half
// of it because production shares one egress address with the rest of the crawl fleet, and
// the measurement cannot see that contention.
//
// Tune it the way ADP's is tuned: downward while boards still 429, upward only while none do.
// Under-shooting leaves boards uncrawled this hour and they keep their last-known state;
// over-shooting re-triggers the storm this exists to stop.
//
// RAISED 2026-09-17 from 200ms (~5 req/s) on the evidence the rule asks for: six hours of
// paced crawling produced ZERO 429s while 231 boards succeeded and the healthy count climbed
// from 2,958 to 3,355. Coverage was the argument for moving — a 50-minute run reached about
// 200 of 5,170 boards before TimeoutStartSec killed it, so a full sweep took roughly two
// days, and the limiter is the wall: the adapter fans out across boards concurrently but
// every request queues behind one shared bucket, so boards-per-run scales with this number
// and nothing else.
//
// 125ms sits at 80% of the measured ceiling rather than at it. That ceiling — 30 of 30
// requests served at 10 req/s — was measured from a CLEAN egress IP, and production shares
// one address with the rest of the fleet, so the last fifth is the margin that measurement
// cannot see. The next move is not a higher number: it is sharding the provider the way
// paylocity is sharded, which buys coverage without asking SmartRecruiters for more.
const (
	smartRecruitersRequestInterval = 125 * time.Millisecond // ~8 req/s
	smartRecruitersRequestBurst    = 2
)

// pacedSmartRecruitersGetter wraps a getter with a fresh limiter shared across one registry
// build, so every board of a run competes for the same token bucket.
func pacedSmartRecruitersGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(smartRecruitersRequestInterval), smartRecruitersRequestBurst),
	}
}

// Workable's widget API (apply.workable.com) meters the crawl as one caller, and every board
// reaches it through that host. Measured on prod 2026-10-19: 3,963 active boards, a static
// timer firing every 3h, two requests per board per run (the listing plus the account
// description) — ~8k requests a run, ~63k a day — fired UNPACED by the runner's 8 board
// workers. Workable began refusing ~30 minutes into each run, after ~1,500 boards: ~3,000
// requests in ~1,800s, so the refusals start near ~1.7 req/s sustained. 18,520 refusals in
// two days, half of them on the description call.
//
// The description call is now asked once per company, not once per run (see
// pipeline.CompanyDescriptionRefreshWindow), so a run is ~one request per board. The scheduler
// splits the provider into 2 shards on a 6h cadence, ~1,982 boards each: at 0.75 req/s a shard
// is ~2,640s (44 min) of requests, spread evenly across the run instead of front-loaded. The
// limiter is per PROCESS, so if both shards ever run at once the provider sees 1.5 req/s —
// still under the ~1.7 req/s the refusals began at, which is why the pace is not higher.
// Tune downward while boards still 429, upward only while none do.
const (
	workableRequestInterval = 1333 * time.Millisecond // ~0.75 req/s
	workableRequestBurst    = 1
)

// pacedWorkableGetter wraps a getter with a fresh limiter shared across one registry build,
// so every Workable board's listing AND description requests in a run draw from one bucket.
func pacedWorkableGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(workableRequestInterval), workableRequestBurst),
	}
}

// doverRequestInterval/doverRequestBurst are NOT measured against a clean ceiling the way
// smartRecruitersRequestInterval's comment documents — freehire crawled zero Dover boards
// before this pacer existed. They are a conservative starting point, set after a live
// incident: a single unpaced run against 31 boards (each firing its own resolve+list+detail
// requests independently) drew 22 HTTP 429s from Dover's API within roughly two seconds
// (2026-09-19). Dover explicitly positions itself as a free ATS for startups, not a platform
// built for a multi-tenant crawl fleet, so this errs slow; revisit only against a real
// ceiling measurement, the same way smartRecruitersRequestInterval eventually was.
const (
	doverRequestInterval = 500 * time.Millisecond // ~2 req/s
	doverRequestBurst    = 1
)

// Dev Global Jobs documents an API rate limit of 120 requests/minute per IP. We pace at
// 600ms (~100 req/min, 1.67 req/s) with a burst of 5 to stay safely under the ceiling across
// list pagination and concurrent detail hydration requests.
const (
	devglobaljobsRequestInterval = 600 * time.Millisecond
	devglobaljobsRequestBurst    = 5
)

// pacedDevGlobalJobsGetter wraps a getter with a fresh limiter shared across one registry build,
// so list pagination and detail hydration in a run compete for the same token bucket.
func pacedDevGlobalJobsGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(devglobaljobsRequestInterval), devglobaljobsRequestBurst),
	}
}

// pacedDoverGetter wraps a getter with a fresh limiter shared across one registry build, so
// every Dover board's resolve/list/detail requests in a run compete for the same token bucket
// — otherwise the runner's per-board concurrency fires them all at once (see
// doverRequestInterval's incident note).
func pacedDoverGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(doverRequestInterval), doverRequestBurst),
	}
}

// pacedADPGetter wraps a getter with a fresh limiter shared across one registry build, so every
// board's listing pages and detail fan-out in a run compete for the same token bucket.
func pacedADPGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(adpRequestInterval), adpRequestBurst),
	}
}

// pacedADPMyJobsGetter paces ADP's OTHER career-site product at the same rate, and on a limiter
// of its OWN. Sharing one bucket between the two would assert that ADP meters them together,
// which nothing here has measured: the 429 storm the rate above answers was observed against
// workforcenow.adp.com, and MyJobs' listing is served from my.adp.com. Halving each product's
// rate for an unobserved shared budget would slow both crawls for a reason we made up; if the
// two do turn out to share a window, the evidence will be MyJobs boards 429ing during the adp
// crawl hour, and the fix is then one limiter passed to both rather than two constructed here.
//
// It carries headers because the MyJobs listing is authorised by an `orgoid` request header
// rather than by query parameters (see adpmyjobs.go).
func pacedADPMyJobsGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(adpRequestInterval), adpRequestBurst),
	}
}

// Phenom People serves ~95 boards, each its own vanity hostname (careers.blizzard.com,
// careers.aegistherapies.com, ...) but — like eightfold's *.eightfold.ai tenants — all fronted
// by the same underlying platform infrastructure, so the run's own aggregate volume across all
// of them can trip one shared window. September 2026: 45 of ~95 boards took a 403 on their
// listing POST during a run, while the same identical request against one of the failing
// hostnames succeeded outside the crawl window minutes later — the shape of a self-inflicted
// burst, not a per-tenant blocklist (a genuinely blocked host would still 403 in isolation).
// Unpaced, defaultConcurrency (8 boards at once) times each board's own paginated fetch is
// enough volume to trip it. The rate is conservative for the same reason every unmeasured pace
// in this file is: the true budget is unknown, and under-shooting only lengthens a run while
// over-shooting re-enters the 403s. Tune from observed convergence (the board_health unhealthy
// count for this provider), downward while boards still 403 in isolation and upward only while
// none do.
const (
	phenomRequestInterval = time.Second // ~1 req/s
	phenomRequestBurst    = 1
)

// pacedPhenomGetter wraps a getter with a fresh limiter shared across one registry build, so
// every board's paginated listing in a run competes for the same token bucket.
func pacedPhenomGetter(c JSONPoster) JSONPoster {
	return rateLimitedJSONPoster{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(phenomRequestInterval), phenomRequestBurst),
	}
}

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
// API's healthy regime; at ~0.5s a page and 2 in flight the whole ~4900-page board still finishes
// well inside the 40-min ingest window. Tune from observed convergence.
const trudvsemMaxInFlight = 2

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

// Teamtailor 403s the crawl in bulk, and the 403 is ours: a "failing" board answers 200 on
// demand from the same IP, and 1207 of 1208 failures carried a timestamp inside the crawl hour.
// The cause is the shape of the adapter — every posting's description is its own page fetch — so
// a run is ~4k listing requests plus ~33k detail ones, and it fired them in 10 minutes: about
// 62 req/s at one career-site vendor. Nearly half the fleet was turned away.
//
// Pacing is the lever that removes the burst rather than moving it: the refusal-retry proxy
// (see refusalRetryProviders) recovered only a quarter of the failures, because a reputation
// 403 follows the volume onto whatever address carries it.
//
// Since FetchNew, a normal run costs a request per NEW posting rather than per posting, so the
// rate matters far less than it did — but the pace stays, sized for the worst case it still has
// to survive: the Fetch fallback, used whenever the pipeline cannot supply a seen set, which
// hydrates everything exactly as before.
//
// The interval is set by the run budget, not by a guess at Teamtailor's window: ~37k requests
// must finish inside the ingest unit's TimeoutStartSec (3000s), so the floor is ~12 req/s and
// this sits above it with room — a full run near 31 minutes. It is the first deliberate rate
// this platform has had; tune from observed convergence, downward while boards are still
// refused and upward only while they are not.
const (
	teamtailorRequestInterval = 50 * time.Millisecond // ~20 req/s
	teamtailorRequestBurst    = 8
)

// rateLimitedTeamtailorGetter paces both of teamtailor's request kinds — its pages (HTML) and
// its jobs.rss feed (XML) — through one limiter, so the feed shares the platform's budget
// rather than adding a second, unpaced stream beside it.
type rateLimitedTeamtailorGetter struct {
	inner   teamtailorHTTP
	limiter waiter
}

func (g rateLimitedTeamtailorGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return g.inner.GetHTML(ctx, url)
}

func (g rateLimitedTeamtailorGetter) GetXML(ctx context.Context, url string, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.inner.GetXML(ctx, url, v)
}

// pacedTeamtailorGetter wraps c with a fresh limiter at teamtailor's rate.
func pacedTeamtailorGetter(c teamtailorHTTP) teamtailorHTTP {
	return rateLimitedTeamtailorGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(teamtailorRequestInterval), teamtailorRequestBurst),
	}
}

// app.idealtraits.com serves every IdealTraits board from one host and meters it per IP, with a
// long penalty: measured 2026-10-20 against posting pages, 4 req/s was served for ~120 requests
// and then refused every request with 429 and a Retry-After of ~240s, while 1.5 req/s held for
// 200 requests (142s) with no refusal. The pace takes that measured clean rate; the hydrating
// crawl keeps a steady-state run near one listing request per board.
const (
	idealtraitsRequestInterval = time.Second * 2 / 3 // 1.5 req/s
	idealtraitsRequestBurst    = 2
)

// rateLimitedIdealTraitsGetter paces both of the adapter's request kinds — the listing XHR (JSON)
// and the posting pages (HTML) — through one limiter, since both hit the same metered host.
type rateLimitedIdealTraitsGetter struct {
	inner   idealtraitsHTTP
	limiter waiter
}

func (g rateLimitedIdealTraitsGetter) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.inner.GetJSONWithHeaders(ctx, url, headers, v)
}

func (g rateLimitedIdealTraitsGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return g.inner.GetHTML(ctx, url)
}

// pacedIdealTraitsGetter wraps c with a fresh limiter at IdealTraits' rate, shared across one
// registry build so every board's listing and posting requests draw on one budget.
func pacedIdealTraitsGetter(c idealtraitsHTTP) idealtraitsHTTP {
	return rateLimitedIdealTraitsGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(idealtraitsRequestInterval), idealtraitsRequestBurst),
	}
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

// Homerun answers every tenant's feed AND vacancy pages from one AWS origin (feed.homerun.co, its
// own *.homerun.co hosts and the employer domains that point at it), and it refuses by RATE, not
// by concurrency. Measured 2026-10-25: a harvest probing 893 feeds at 4 in flight from fsn drew
// 429s within 8 seconds and then a 403 on every request from that address for over an hour, while
// the same 874 feeds at 4 in flight from a laptop (~0.5s round trips, so ~8/s) all answered; a
// sweep of 2,499 vacancy pages at 4 in flight drew 392 refusals once it had run a few minutes.
// So every request the adapter makes, feed and page alike, draws on one provider-wide budget of 2
// a second: a first crawl of ~500 feeds and ~2,500 pages is ~25 minutes, and a refresh run, which
// only reads the feeds and the few new pages, a few minutes.
const (
	homerunRequestInterval = 500 * time.Millisecond
	homerunRequestBurst    = 2
)

// pacedHomerun routes both of Homerun's request kinds through one shared limiter.
type pacedHomerun struct {
	homerunGetter
	limiter waiter
}

func (g pacedHomerun) GetXML(ctx context.Context, url string, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.homerunGetter.GetXML(ctx, url, v)
}

func (g pacedHomerun) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return g.homerunGetter.GetHTML(ctx, url)
}

// pacedHomerunGetter wraps a getter with a fresh limiter shared across one registry build.
func pacedHomerunGetter(c homerunGetter) homerunGetter {
	return pacedHomerun{homerunGetter: c, limiter: rate.NewLimiter(rate.Every(homerunRequestInterval), homerunRequestBurst)}
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

// api.higherme.com sits behind an AWS load-balancer WAF with a rate-based rule that answers the
// whole IP 403 once tripped. The 2026-10-20 spike tripped it after roughly 70 requests in about
// six minutes (resolving postings one by one), and the block lifted within a few minutes of
// going quiet — the shape of a per-window request budget, whose exact size is unmeasured. A
// board is one request (two for an empty one), so a gentle pace costs little: a few hundred
// brands crawl in well under an hour at one request every three seconds, ~100 per five minutes
// at most, with the shared client's retries spending the headroom below the observed trip.
const (
	highermeRequestInterval = 3 * time.Second
	highermeRequestBurst    = 1
)

// pacedHigherMeGetter wraps a getter with a fresh limiter shared across one registry build, so
// every board's listing pages and brand lookups in a run compete for the same token bucket.
func pacedHigherMeGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(highermeRequestInterval), highermeRequestBurst),
	}
}

// apponeRequestInterval paces every AppOne request of a run — the portal listings on
// jobs.appone.com and the posting details on apply.appone.com sit behind one Azure Front Door
// edge that turns a fast client away with a bare 403, not a 429, so the shared client's
// per-host hold never engages. Measured 2026-10-20 from one IP: ~100 requests back to back
// degraded within a minute to 38 refusals in 40, for pooled and single-use connections alike,
// and recovered after a pause. At one request a second a 100-posting portal and its details
// read cleanly, but refusals came back in bursts a few minutes in — and did so at one request
// every two seconds too (0-14 per 20 over seven minutes), so no pace measured removes them;
// this one keeps them rare enough for the adapter's retry and the next run to absorb (see the
// appone section of AGENTS.md). Tune downward while many details come back Unreadable.
const (
	apponeRequestInterval = time.Second
	apponeRequestBurst    = 1
)

// pacedAppOneGetter wraps a getter with a fresh limiter shared across one registry build, so
// every AppOne board's listing and detail requests in a run draw from one bucket.
func pacedAppOneGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(apponeRequestInterval), apponeRequestBurst),
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
