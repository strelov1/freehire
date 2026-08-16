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

// portal-api.emagine.org serves a single detail request in ~0.9s and tolerates a burst (80
// back-to-back at 8-way all returned 200), but a whole-catalogue hydration — 1025 details in one
// run — starts refusing CONNECTIONS near the end: `dial tcp: i/o timeout`, already past the
// client's own three attempts, on ~1% of postings clustered in the run's tail. So the trigger is
// sustained concurrency over a long run, not rate, and the loss is permanent: a posting ingested
// list-only is seen on the next crawl and never re-fetches its detail. Four in flight halves the
// pressure while keeping a full first crawl inside the ingest window; later crawls hydrate only
// new postings, so the cap costs nothing in steady state. Tune from the observed loss rate.
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
