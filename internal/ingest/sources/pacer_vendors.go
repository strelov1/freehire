package sources

import (
	"context"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// Phenom People serves hundreds of boards, each its own vanity hostname but all fronted by the
// same platform infrastructure, so the run's aggregate volume can trip one shared window. A
// failing hostname succeeded minutes later outside the crawl, so the 403s were self-inflicted
// bursts, not a per-tenant blocklist. Unpaced, defaultConcurrency boards times each board's
// paginated fetch was enough to trip it. The burst that tripped it was ~64 requests IN FLIGHT; a
// pace caps starts, and 3 req/s keeps only a few in flight. Tune from the board_health unhealthy
// count: downward while boards still 403 in isolation, upward only while none do.
const (
	phenomRequestInterval = time.Second / 3 // ~3 req/s
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

// Teamtailor 403s the crawl in bulk, and the 403 is ours: a "failing" board answers 200 on demand
// from the same IP, and the failures sit inside the crawl hour. Every posting's description is its
// own page fetch, so an unpaced run fired tens of thousands of requests in minutes at one
// career-site vendor. Pacing removes the burst rather than moving it: the refusal-retry proxy
// (see refusalRetryProviders) recovered only a quarter, because a reputation 403 follows the
// volume onto whatever address carries it.
//
// Since FetchNew a normal run costs a request per NEW posting, but the pace stays, sized for the
// Fetch fallback (used whenever the pipeline cannot supply a seen set), which hydrates everything.
//
// The interval is set by the run budget, not a guess at Teamtailor's window: ~37k requests must
// finish inside the ingest unit's TimeoutStartSec (3000s), so the floor is ~12 req/s and this
// sits above it. Tune downward while boards are still refused, upward only while they are not.
const (
	teamtailorRequestInterval = 50 * time.Millisecond // ~20 req/s
	teamtailorRequestBurst    = 8
)

// rateLimitedTeamtailorGetter paces both request kinds, pages (HTML) and the jobs.rss feed
// (XML), through one limiter so the feed shares the platform's budget.
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

// app.idealtraits.com serves every IdealTraits board from one host and meters it per IP with a
// long penalty: 4 req/s was served for ~120 requests and then every request got 429 with a
// Retry-After of ~240s, while 1.5 req/s held clean. The pace takes the clean rate.
const (
	idealtraitsRequestInterval = time.Second * 2 / 3 // 1.5 req/s
	idealtraitsRequestBurst    = 2
)

// rateLimitedIdealTraitsGetter paces the listing XHR (JSON) and the posting pages (HTML) through
// one limiter, since both hit the same metered host.
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

// Homerun answers every tenant's feed AND vacancy pages from one AWS origin (feed.homerun.co,
// *.homerun.co and employer domains pointing at it), and refuses by RATE, not concurrency: a
// harvest at 4 in flight drew 429s within seconds and then a 403 on every request from that
// address for over an hour, while a slower client succeeded. So every request, feed and page
// alike, draws on one provider-wide budget.
//
// 2 a second was also not enough from the datacenter address: most scheduled runs were refused
// minutes in, so no rate was shown safe, only 2 a second shown unsafe. The budget is halved to
// 1 a second, and what happens after a refusal is homerunEgress's: the rest of the run moves to
// the rotating proxy, or stops.
const (
	homerunRequestInterval = time.Second
	homerunRequestBurst    = 1
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

// api.higherme.com sits behind an AWS load-balancer WAF with a rate-based rule that 403s the whole
// IP once tripped (after roughly 70 requests in six minutes), lifting a few minutes after going
// quiet: a per-window budget of unmeasured size. A board is one request (two when empty), so a
// three-second admission is not a measured safe rate. Every wire attempt, including retries,
// must draw a token; see highermeAttemptGetter and providers/higherme.md.
const (
	highermeRequestInterval = 3 * time.Second
	highermeRequestBurst    = 1
)

// pacedHigherMeGetter wraps a getter with a fresh limiter shared across one registry build, so
// every board's listing pages and brand lookups in a run compete for the same token bucket.
func pacedHigherMeGetter(c JSONGetter) JSONGetter {
	limiter := rate.NewLimiter(rate.Every(highermeRequestInterval), highermeRequestBurst)
	if client, ok := c.(*Client); ok && client != nil {
		return highermeAttemptGetter(client, limiter)
	}
	return rateLimitedJSONGetter{inner: c, limiter: limiter}
}

// apponeRequestInterval paces every AppOne request of a run. The portal listings (jobs.appone.com)
// and posting details (apply.appone.com) sit behind one Azure Front Door edge that turns a fast
// client away with a bare 403, not a 429, so the shared client's per-host hold never engages.
// Bursts of refusals return a few minutes in even at one request every two seconds, so no pace
// removes them; this one keeps them rare enough for the adapter's retry and the next run to
// absorb (see the appone section of AGENTS.md). Tune downward while many details come back
// Unreadable.
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

// recruiting.ultipro.com (and other UKG Pro Recruiting hosts) front every board with one nginx edge
// that 403s the project's own User-Agent outright (see crawlerUASource), and the adapter
// re-fetches every posting's detail page on every run (not a HydratingSource), so volume is
// listings plus postings against one shared edge. No rate-triggered refusal has been observed, so
// the pace is precautionary like ADP's: over-shooting risks teaching an edge that already filters
// hard to drop the Googlebot string too. Tune from the board_health unhealthy count.
const (
	ukgRequestInterval = 200 * time.Millisecond // ~5 req/s
	ukgRequestBurst    = 2
)

// rateLimitedUKGHTTP paces the listing JSON POST and the detail-page GET through one limiter,
// since both hit the same shared edge.
type rateLimitedUKGHTTP struct {
	inner   ukgHTTP
	limiter waiter
}

func (g rateLimitedUKGHTTP) PostJSON(ctx context.Context, url string, body, v any) error {
	if err := g.limiter.Wait(ctx); err != nil {
		return err
	}
	return g.inner.PostJSON(ctx, url, body, v)
}

func (g rateLimitedUKGHTTP) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return g.inner.GetHTML(ctx, url)
}

// pacedUKGHTTP wraps c with a fresh limiter at ukg's rate, shared across one registry build.
func pacedUKGHTTP(c ukgHTTP) ukgHTTP {
	return rateLimitedUKGHTTP{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(ukgRequestInterval), ukgRequestBurst),
	}
}
