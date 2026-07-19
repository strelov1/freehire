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

// careers-page.com rate-limits by a per-IP request budget per time window, so a full run must
// hold its aggregate request rate under it (proxy egress and a narrow worker pool cap the
// burst, not the total-per-window — see the careerspage-request-pacer change). The interval is
// conservative because the true budget is unknown: under-shooting only lengthens a run, while
// over-shooting re-introduces the 429 starvation. Tune from observed convergence.
const (
	careerspageRequestInterval = 800 * time.Millisecond // ~1.25 req/s
	careerspageRequestBurst    = 2
)

// pacedCareerPageGetter wraps a getter with a fresh limiter shared across one registry build,
// so all of careerspage's requests in a run are paced under careers-page.com's window budget.
func pacedCareerPageGetter(c HTMLGetter) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(careerspageRequestInterval), careerspageRequestBurst),
	}
}

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

// pacedVagasGetter wraps a getter with a fresh limiter shared across one registry build, so all
// of vagas's requests in a run stay under vagas.com.br's per-IP window on the single proxy IP.
func pacedVagasGetter(c HTMLGetter) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(vagasRequestInterval), vagasRequestBurst),
	}
}

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

// pacedClinchGetter wraps a getter with a fresh limiter shared across one registry build, so all
// of clinch's detail requests in a run stay under ClinchTalent's per-IP AWS-WAF challenge window.
func pacedClinchGetter(c HTMLGetter) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(clinchRequestInterval), clinchRequestBurst),
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

// hh.ru egresses through the single proxy IP (its detail pages 403 the direct datacenter IP), and
// its per-vacancy detail fan-out is large — thousands of ~1 MB pages across the seeded roles. Fired
// unpaced at defaultDetailWorkers concurrency, that burst 429s the proxy IP and ~2/3 of details
// fall back to list-only (which never back-fill, since a seen posting skips detail). Pacing the
// aggregate rate — not the worker pool — holds it under the proxy window so nearly every detail
// lands. The interval is a middle ground: fast enough to finish a full role sweep inside the
// ingest unit's TimeoutStartSec, gentle enough to stop the 429s. Tune from observed convergence.
const (
	hhRequestInterval = 250 * time.Millisecond // ~4 req/s
	hhRequestBurst    = 4
)

// pacedHHGetter wraps a getter with a fresh limiter shared across one registry build, so all of
// hh.ru's search and detail requests in a run stay under the proxy IP's per-window budget.
func pacedHHGetter(c HTMLGetter) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(hhRequestInterval), hhRequestBurst),
	}
}
