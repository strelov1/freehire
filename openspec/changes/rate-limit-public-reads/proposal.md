## Why

**The public read API has no rate limit at all.** Not a generous one — none.
Every one of the eighteen limiters built on `internal/ratelimit` guards an auth
route, a write, or an LLM spend. `nginx` applies `limit_req` to `location /`
(the SvelteKit pages, 10 r/s per IP) and **not** to `location /api/`, and the
`freehire-limit-req` fail2ban jail reads the `limit_req` rejections that only
that location produces. Measured on production: 134 IPs are currently banned
across the nginx jails, and **zero** of their requests ever touched
`/api/v1/`. An API client cannot be banned, because nothing observes it.

One consumer already lives in that gap. `ManyApplyAssist/6.0` holds a steady
~180 requests per minute — 59% of all API traffic in a 4.6-hour window — and
**100% of it lands on `/api/v1/agent/jobs/search`**, the most expensive endpoint
we serve (833 KB and ~1.3 s at `limit=100`, against 123 KB and ~0.55 s for the
ordinary search). It is not misbehaving; it simply has no ceiling to respect and
no way to learn one, because we publish neither a limit nor a header.

That was already worth fixing. #2085 and #2092 made it urgent: `robots.txt` and
`llms.txt` now actively tell crawlers to stop scraping the HTML and call the API
instead — steering traffic off the rate-limited path onto the unlimited one.
That advice is right (one JSON call is cheaper than an SSR render and returns
more) and it must not ship without the ceiling it assumes.

## What Changes

- **Public read endpoints get two budgets**, split by measured cost rather than
  by path: the cheap reads (`/jobs/search`, `/jobs/facets`, `/jobs/{slug}`,
  `/jobs/{slug}/similar`, `/companies`, `/companies/{slug}`, `/geo/cities`) at
  600/min, and `/agent/jobs/search` alone at 300/min. Both sit above today's
  measured per-IP peaks (258/min and 184/min), so no current caller is cut off
  on the day this ships.
- **Every rate-limited response carries `X-RateLimit-Limit`,
  `X-RateLimit-Remaining` and `X-RateLimit-Reset`.** This is the half that makes
  a limit legible instead of merely punitive, and it applies to all eighteen
  existing limiters too — today a caller who exhausts the login limiter gets a
  bare 429 with no `Retry-After` guidance until the very moment of rejection.
- **Loopback and private-network callers skip the limiter entirely.** Without
  this the change is not merely wrong, it is an outage: SSR reaches the API at
  `http://127.0.0.1:8081`, bypassing nginx and sending no `X-Real-IP`, so
  `c.IP()` returns `127.0.0.1` for every server-rendered page and the whole site
  would share one bucket.
- **`openapi.yaml` documents the limits and the headers**, closing the gap a
  third-party integrator named in writing: *"There is no SLA and no rate-limit
  header of any kind (verified: no `X-RateLimit-*`, no `Retry-After`)."*

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `api-rate-limiting`: gains client-visible budget headers, a public-read
  ceiling, and a trusted-caller exemption. The shared-backend, key-namespacing
  and fail-open contracts are unchanged.
- `api-documentation`: the published schema states the limits and the headers.

## Impact

- `internal/ratelimit/throttler.go` — `Throttler.Allow` returns a `Decision`
  carrying `Remaining`/`Limit`/`ResetAfter` instead of discarding what
  `redis_rate` already computes; `Middleware` sets the headers and skips
  trusted peers.
- `internal/ratelimit/redis.go` — populate the new fields.
- `internal/ratelimit/{keys,middleware,redis}_test.go` — four test fakes
  implement the widened interface.
- `internal/handler/{search,jobs,companies,geo}.go` — attach the two limiters.
- `web/static/openapi.yaml` — document limits and headers.
- No schema change, no migration, no reindex, no new dependency.

**Deliberately not done here.** No tiering: an API key does not buy a higher
ceiling. That is a monetization decision, and building the mechanism before the
decision would be infrastructure ahead of need. The seam is free when it is
wanted — the limiter already keys by user when a caller is authenticated, so a
per-tier budget is a lookup where a constant is today.

**Deliberately not done here.** No fail2ban jail for the API. A ban must come
*after* a published ceiling and a header telling callers what it is, never
before: the first client a 24-hour ban would catch is the conscientious
integrator who simply never had a number to respect.
