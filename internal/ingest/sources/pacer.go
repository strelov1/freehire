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

// rateLimitedHTMLGetter wraps an HTMLGetter with a shared limiter so its aggregate GetHTML rate
// stays under the limit, independent of worker concurrency. One instance, one token bucket,
// shared by every board and by listing and detail paths.
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

// pacedHTMLGetter wraps a getter with a fresh limiter (one token per interval) shared across one
// registry build. The per-provider interval/burst constants below carry the rationale.
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

// careers-page.com rate-limits by a per-IP request budget per time window, so a full run must hold
// its aggregate rate under it; the pool and proxy egress cap only the burst. The interval is
// conservative because the true budget is unknown: under-shooting only lengthens a run,
// over-shooting brings back 429 starvation. Tune from observed convergence.
const (
	careerspageRequestInterval = 800 * time.Millisecond // ~1.25 req/s
	careerspageRequestBurst    = 2
)

// vagas.com.br rate-limits by a per-IP request budget: an unpaced crawl 429s the single egress
// proxy IP and then 429s even spaced requests during the penalty window. The detail pool bursts to
// defaultDetailWorkers, so the pacer, not the pool, holds the aggregate rate. More conservative
// than careerspage's since vagas 429'd hard; tune from observed convergence.
const (
	vagasRequestInterval = time.Second // ~1 req/s
	vagasRequestBurst    = 1
)

// www.workstream.us is served by AWS API Gateway and meters by request RATE per IP, on every path.
// A short sample reads the knee near 1 req/s, but a long crawl at that pace was still refused:
// the budget is a rate held over the whole run, not a burst the first refusal announces.
//
// Two things make the pace worse than the number it names: the penalty outlives the burst that
// earned it, and a refused request RETRIES three times beneath this limiter (the backoff lives in
// the client), so one paced token can become three requests exactly when the platform is
// refusing. That feedback loop is why the pace sits at half the observed ceiling.
//
// The detail fan-out bursts to defaultDetailWorkers, so the pacer holds the rate. Tune downward
// while boards still fail with 429.
const (
	workstreamRequestInterval = 2 * time.Second // ~0.5 req/s
	workstreamRequestBurst    = 1
)

// Abacus Umantis tenants (recruitingapp-<n>[.de].umantis.com) are separate hosts on one origin
// that rate-limits per client IP across all of them. Unpaced, the scheduler's parallel boards
// (8 detail fetches each) drew 429s until whole boards read no detail at all (from 2026-11-16:
// 7-9 failed boards a run, Stadt Leverkusen among them); a 48-way burst from the crawl host got
// 57 of 60 answered 429, while one board's 8-way fetch of 25 pages was served in full. The run
// is ~850 requests, so 2 req/s finishes it in ~7 minutes. Tune from the run's unreadable count.
const (
	umantisRequestInterval = 500 * time.Millisecond // 2 req/s across every Umantis board
	umantisRequestBurst    = 2
)

// ClinchTalent fronts detail pages with a rate-based AWS-WAF Challenge: a cold IP gets a handful of
// clean pages before the WAF flips to a 202 challenge with a long per-IP penalty. clinch fetches
// one detail page per new posting, so the aggregate rate, not the pool, must stay under that
// window. Deliberately gentle since the budget is unknown: over-shooting trips the WAF and
// latches clinch to sitemap-only for the rest of the run. Tune from the description-fill rate.
const (
	clinchRequestInterval = 1500 * time.Millisecond // ~0.67 req/s
	clinchRequestBurst    = 1
)

// energyjobline.com's robots.txt declares Crawl-delay: 10 and its sitemap lists a huge catalogue,
// so honouring it literally would take weeks per crawl. This pace is a policy compromise, faster
// than asked and slower than unpaced, not a measured block-avoidance ceiling (contrast clinch).
// Revisit if the site starts refusing requests at this rate.
const (
	energyjoblineRequestInterval = 1500 * time.Millisecond // ~0.67 req/s
	energyjoblineRequestBurst    = 1
)

// dynamitejobs.com's robots.txt declares no Crawl-delay and disallows only /api/ and the account
// areas. The first run reads every posting page once (~500, measured 2026-11-17) and later runs
// only the new ones, so a gentle 1 req/s costs ~10 minutes once. Chosen for politeness, not a
// measured block threshold - none appeared over 60 sequential requests at 0.5s.
const (
	dynamitejobsRequestInterval = time.Second
	dynamitejobsRequestBurst    = 1
)

// airswift.com 429s detail pages only under sustained volume, and the penalty outlives the burst
// (cleared after ~15 minutes idle). Short samples at 2s and 5s per request looked clean, yet real
// paced crawls at both still 429'd every request: the budget is metered over a window longer than
// any hand-run sample, so short samples understate it (as with workstream). 15s is chosen to
// get well outside that window, not a proven ceiling. If a full paced crawl at this rate still
// 429s, move the provider to the deferred group rather than guessing again.
const (
	airswiftRequestInterval = 15 * time.Second // ~0.067 req/s
	airswiftRequestBurst    = 1
)

// rateLimitedJSONGetter is the JSON-GET counterpart of rateLimitedHTMLGetter: one shared limiter
// for every board, listing page and detail request.
//
// Distinct from concurrencyLimitedJSONGetter (pacer_limits.go), which caps requests in FLIGHT for
// an API that degrades under concurrent load; this caps the request rate, for an API that meters
// requests per unit time.
type rateLimitedJSONGetter struct {
	inner   JSONGetter
	limiter waiter
}

// GetJSON blocks on the limiter before delegating; a cancelled context surfaces as the Wait error.
func (g rateLimitedJSONGetter) GetJSON(ctx context.Context, url string, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.inner.GetJSON(ctx, url, v)
}

// rateLimitedHeaderJSONGetter is rateLimitedJSONGetter for an adapter whose requests carry
// headers. Every step goes through this one method (nil headers when none are needed), so one
// limiter is the whole budget.
type rateLimitedHeaderJSONGetter struct {
	inner   HeaderJSONGetter
	limiter waiter
}

// GetJSONWithHeaders blocks on the limiter before delegating; a cancelled context surfaces as the Wait error.
func (g rateLimitedHeaderJSONGetter) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.inner.GetJSONWithHeaders(ctx, url, headers, v)
}

// join.com meters by RATE, not concurrency: an unpaced crawl's refusals looked like a concurrency
// limit, but holding the rate steady and varying only the worker count clears it. The knee sits
// between 2 and 3 req/s, and the pace takes the clean side of it.
//
// Even 2 req/s left a residue: refusals accelerated through a run, which looks like a cumulative
// budget layered on the per-second one and invisible in short samples. 1.5 req/s buys headroom;
// that pace needed joinShards raised from 4 to 5 (TestJoinPaceFitsTheRunBudget pins the
// arithmetic).
const (
	joinRequestInterval = time.Second * 2 / 3 // 1.5 req/s
	joinRequestBurst    = 2
)

// rateLimitedJSONPoster is the JSON-POST counterpart of rateLimitedHTMLGetter, for detail
// endpoints that are not GETs (SEEK's is a GraphQL POST).
type rateLimitedJSONPoster struct {
	inner   JSONPoster
	limiter waiter
}

// PostJSON blocks on the limiter before delegating; a cancelled context surfaces as the Wait error.
func (p rateLimitedJSONPoster) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.PostJSON(ctx, url, body, v)
}

// www.seek.com.au/graphql enforces a per-IP request BUDGET rather than degrading under load: an
// unpaced crawl fired thousands of detail POSTs in under two minutes and was 429'd on nearly all,
// while a single request after idle returned 200. A refusal that clears on idle is a window, not
// saturation, hence a rate limiter and not the in-flight cap trudvsem and emagine use.
//
// The rate is far below the refused one and conservative since the budget is unknown. The first
// backfill spans a few runs, which is safe only because seek defers a posting it could not
// hydrate instead of storing it body-less. Tune from the description-fill rate.
const (
	seekDetailInterval = 500 * time.Millisecond // ~2 req/s
	seekDetailBurst    = 2
)

// pacedSeekPoster wraps a poster with a fresh limiter shared across one registry build. Only the
// detail path is wrapped: seek's search listing has never been refused.
func pacedSeekPoster(c JSONPoster) JSONPoster {
	return rateLimitedJSONPoster{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(seekDetailInterval), seekDetailBurst),
	}
}

// The SEEK proxy lane (see seekDetailNetwork) runs on its OWN bucket: it is a different IP, and
// SEEK's budget is per IP. Its pace is half the direct lane's because the proxy is one static
// address that every proxiedProviders entry egresses through, and losing it to a SEEK ban would
// leave the network with no lane. 1 req/s still spends the whole seekDetailRunBudget inside half
// a run's timeout.
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

// Eightfold rate-limits an IP to a request budget per window, and its crawl egresses through the
// single shared proxy IP with other providers (proxiedProviders), so the concurrency cap and
// retry backoff are not enough: the aggregate request START rate needs its own ceiling. Unpaced,
// listing calls 403'd (a window spent by the run's own volume, not an IP blocklist). The interval
// matches vagas's, the gentlest pace on this proxy, since the budget and the other providers'
// share of it are unknown. Tune from the board_health unhealthy count: downward while boards
// fail their listing call, upward only while none do.
const (
	eightfoldRequestInterval = time.Second // ~1 req/s
	eightfoldRequestBurst    = 1
)

// pacedEightfoldGetter wraps a getter with a fresh limiter shared across one registry build;
// listing and detail both hit the same rate-limited edge.
func pacedEightfoldGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(eightfoldRequestInterval), eightfoldRequestBurst),
	}
}

// ADP Workforce Now serves thousands of boards from the single host workforcenow.adp.com, and the
// adapter re-lists and re-fetches EVERY requisition's detail on every run (no HydratingSource
// seen-skip yet), so volume is large and grows with the catalogue. Unpaced, most boards took a
// 429 on the listing call alone: a window spent by the run's own aggregate rate against one
// shared host, not an IP blocklist. The rate is deliberately below what covering the whole
// catalogue in one window needs: under-shooting leaves boards uncrawled this run (they keep
// their last-known state), over-shooting re-triggers the 429 storm. A true seen-skip is the
// real fix for run time. Tune from the board_health unhealthy count: downward while boards
// still 429, upward only while none do.
const (
	adpRequestInterval = 200 * time.Millisecond // ~5 req/s
	adpRequestBurst    = 2
)

// SmartRecruiters meters its public postings API per caller, and all of its boards reach it
// through one host, the same shape as ADP. Unpaced, the fleet spent most of its budget on 429
// retries and thousands of boards sat failed at any moment.
//
// The ceiling is at least 10 req/s from a clean egress IP; the pace sits at 80% of it because
// production shares one egress address with the rest of the fleet, which that measurement cannot
// see. Boards-per-run scales with this number alone: every request queues behind one shared
// bucket. The next move for coverage is sharding the provider like paylocity, not a higher rate.
//
// Tune like ADP: downward while boards still 429, upward only while none do.
const (
	smartRecruitersRequestInterval = 125 * time.Millisecond // ~8 req/s
	smartRecruitersRequestBurst    = 2
)

// pacedSmartRecruitersGetter wraps a getter with a fresh limiter shared across one registry build.
func pacedSmartRecruitersGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(smartRecruitersRequestInterval), smartRecruitersRequestBurst),
	}
}

// Workable's widget API (apply.workable.com) meters the crawl as one caller, and every board
// reaches it through that host. Fired unpaced by the runner's 8 board workers, Workable started
// refusing about 30 minutes into each run, so refusals begin near ~1.7 req/s sustained.
//
// The description call is asked once per company, not once per run (see
// pipeline.CompanyDescriptionRefreshWindow), so a run is ~one request per board. The scheduler
// splits the provider into 2 shards on a 6h cadence; the limiter is per PROCESS, so if both
// shards run at once the provider sees twice the pace, which must still stay under ~1.7 req/s;
// that is why the pace is not higher. Tune downward while boards still 429, upward only while
// none do.
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

// doverRequestInterval is a conservative starting pace, not a measured ceiling: an unpaced run
// over a few dozen boards (each firing its own resolve+list+detail requests) drew a burst of 429s
// within seconds. Dover positions itself as a free ATS for startups, not a multi-tenant crawl
// target, so this errs slow; revisit only against a real ceiling measurement.
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

// pacedDoverGetter wraps a getter with a fresh limiter shared across one registry build, so every
// Dover board's resolve/list/detail requests compete for one bucket instead of firing at once.
func pacedDoverGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(doverRequestInterval), doverRequestBurst),
	}
}

// pacedUnstopGetter holds every Unstop request to one limiter at the site's Crawl-delay.
func pacedUnstopGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(unstopRequestInterval), 1),
	}
}

// pacedJumpitGetter holds every Jumpit request, listing and detail fan-out, to one limiter (see
// jumpitRequestInterval).
func pacedJumpitGetter(c JSONGetter) JSONGetter {
	return rateLimitedJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(jumpitRequestInterval), jumpitRequestBurst),
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

// pacedADPMyJobsGetter paces ADP's OTHER career-site product at the same rate on a limiter of its
// OWN: the 429 storm was observed against workforcenow.adp.com, MyJobs is served from my.adp.com,
// and nothing shows the two share a window. If they do, MyJobs boards will 429 during the adp
// crawl hour, and the fix is one limiter passed to both.
//
// It carries headers because the MyJobs listing is authorised by an `orgoid` request header
// (see adpmyjobs.go).
func pacedADPMyJobsGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(adpRequestInterval), adpRequestBurst),
	}
}
