## Context

careers-page.com enforces a per-IP request budget over a rolling time window; exceeding
it returns HTTP 429 (even on the single-request listing, once tripped). The shipped
mitigations — proxy egress (#791, a fresh IP) and `careerspageDetailWorkers = 2` (#792,
a narrow detail pool) — cap the instantaneous burst but not the *total* requests per
window. In a full `careerspage` run the boards crawl sequentially over one shared proxy
IP (`ApplyProxyEgress` rebuilds the adapter over a single `NewProxyClient`), so an
earlier/large board (David Joseph, ~72 postings) spends the window and later boards
(Alcor, ~34) starve. Concurrency limits the fan-out width; only pacing the aggregate
request *rate* keeps a whole run under the window.

Current wiring: `All(c)` registers `NewCareerPage(c)` on the shared client; when
`SOURCES_PROXY_URL` is set, `ApplyProxyEgress` swaps in `proxiedProviders["careerspage"]`
built over the proxied client. The adapter fans out detail fetches via
`fetchDetails(locs, careerspageDetailWorkers, ...)`.

## Goals / Non-Goals

**Goals:**
- One full careerspage run collects every posting across all boards, without 429
  starvation of later boards.
- The rate cap is aggregate (shared across boards + listing + detail in a run) and
  independent of worker count.
- Reusable seam: any provider could adopt the same pacing later with one wiring line.

**Non-Goals:**
- Not auto-discovering careers-page.com's true budget — the rate is a conservative
  constant, tunable later from observed convergence.
- Not applying pacing to any other provider now (eightfold/djinni/2gis stay as-is).
- Not changing careerspage parsing, pagination, dedup identity, or `careerspageDetailWorkers`.

## Decisions

**1. A rate-limited `HTMLGetter` decorator, not per-adapter delays.**
Introduce a small type wrapping an `HTMLGetter` (the one interface careerspage uses — both
its paginated listing and every detail fetch go through `GetHTML`) plus a limiter. `GetHTML`
blocks on the limiter before delegating. One decorator instance carries one limiter, so all
of careerspage's requests — across every board and both the listing and detail paths — share
the same token bucket. Scoping to `HTMLGetter` (not the full 10-method `HTTPClient` interface)
keeps it to the single method careerspage actually calls instead of nine wrappers of pure
boilerplate; widening to other request kinds later is a trivial addition when a JSON/XML
provider needs pacing. It stays provider-agnostic (any HTML-detail adapter can wrap its
getter) and is testable in isolation via an injected fake waiter asserting the gate fires
before each fetch.
- *Alternative rejected — full `HTTPClient` decorator:* nine method wrappers careerspage never
  calls, all boilerplate, for no present benefit.
- *Alternative rejected — a `time.Sleep` inside `careerspage.detail`:* couples pacing to one
  adapter, doesn't cover the listing path, and races under the worker pool.
- *Alternative rejected — an optional limiter field on the shared `Client`:* central and
  clean, but modifies the client every provider shares for a one-provider need, and
  `GetStream` bypasses the central `do` path anyway.

**Testability of the gate.** The decorator holds a minimal `waiter` interface
(`Wait(context.Context) error`), which `*rate.Limiter` satisfies; tests inject a fake that
counts calls and asserts `Wait` fires before each delegated `GetHTML` without timing flake.

**2. Provider-scoped, constructed at registry-build time.**
Wrap careerspage's client once where the adapter is registered, so the limiter's lifetime
matches a registry (one worker run). Mirror the `proxiedProviders` opt-in shape: careerspage
is the only opted-in provider, but the helper takes any `HTTPClient`, so adding another is
one line. The wrap must sit *outside* the proxy swap so a proxied careerspage is still paced.
- *Alternative rejected:* a global process-wide limiter — wrong lifetime and couples
  unrelated providers.

**3. Keep `careerspageDetailWorkers = 2`.**
With the pacer governing aggregate rate, workers only trade latency (a worker blocked in
`limiter.Wait` while another issues its request). 2 is a safe default; revisit only if runs
are too slow. Leaving it unchanged keeps this change to the pacing seam alone.

**4. Conservative starting rate.**
Start around ~1 request/second with a small burst (e.g. `rate.Every(800ms)`, burst 2) — the
true budget is unknown, and under-shooting only makes a run longer, while over-shooting
re-introduces the 429 starvation. Expose the rate as a named constant so tuning is a one-line
change after observing convergence.

## Risks / Trade-offs

- **Rate mis-tuned (too high) → 429s return** → start conservative; the standard client's
  existing 429-retry backoff still absorbs the occasional overshoot; tune down from observed
  logs.
- **Longer run time** (bounded rate × ~100+ requests ≈ a couple of minutes) → confirm the
  `freehire-ingest@careerspage` unit's `TimeoutStartSec` has headroom before/at deploy; the
  run is still far under any reasonable timeout.
- **New dependency `golang.org/x/time/rate`** → tiny, widely used, effectively std-adjacent;
  acceptable.
- **Limiter shared only within one registry build** → correct: each worker run gets a fresh
  budget window, matching careers-page.com's per-window reset.

## Migration Plan

1. Ship behind no flag (pacing is always-on for careerspage; it only slows requests).
2. Deploy via the standard blue/green `release.sh`; workers pick it up on the next run.
3. Observe: run `careerspage` ingest, confirm `failed=0`, Alcor climbs toward ~34 and David
   Joseph stays full in a single run.
4. Rollback: revert the one-line wiring (careerspage falls back to the un-paced client);
   no data migration.

## Open Questions

- Exact rate constant — pick conservatively now, refine from the first paced run's logs.
