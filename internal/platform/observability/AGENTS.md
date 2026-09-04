# internal/platform/observability — Sentry Error Tracking

Opt-in Sentry across all three surfaces, env-gated.

## Backend Server (`cmd/server`)

- `observability.Init(dsn, environment)` wraps `sentry.Init` with defaults (`SendDefaultPII:false`, tracing off — **errors-only**), returns a `flush`.
- Empty DSN = **no-op** (app runs unchanged). Malformed DSN = **fatal** (fail-fast).
- `sentryfiber` middleware registered **after** `recover.New` so deferred capture reports panic *with a stack* before `recover.New` renders standard 500 (`Repanic:true`).
- `handler.RenderError` reports **only** fall-through unexpected 500 to request hub — routine 4xx / `pgx.ErrNoRows`→404 / FK-violation→404 are never reported. Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- **Streamed responses need their own reporting.** `RenderError` only ever sees errors a handler *returns*, and an SSE handler returns `nil` before its body writer runs — so a failure inside the stream reports nothing, while the access log records the `200` the stream opened with. `handler.reportStreamFault` closes that gap: it takes a **clone** of the request hub captured before the ctx is released, and defers to the same `classify` policy so a reader who walked away is not filed as a fault. Both SSE surfaces call it — the fit stream (`match_analysis_stream.go`) and the assistant turn (`assistant.go`). Any new streaming endpoint has this blind spot until it does the same.
- **Bound every SSE write** (`sseWriteTimeout`). Both streams set a write deadline before *each* write rather than clearing it: fasthttp runs the stream writer on its own goroutine while the serving goroutine arms the server's `WriteTimeout`, so setting once races and loses about half the time — and a *cleared* deadline is forever, letting a reader that stopped reading pin the goroutine for the life of the process.

## Workers

- `observability.Init` lives in `worker.Bootstrap` (flush folded into `cleanup`).
- Every cron worker's `main` uses `worker.Main(run)` — deferred `capturePanic` captures + flushes + re-panics so short-lived run-once process still delivers fatal panic before crashing non-zero.
- `harvest-*`/`gen-contracts` **dev tools are out of scope** (no Bootstrap).

## Frontend (`web/`)

- `@sentry/sveltekit` in `hooks.client.ts`/`hooks.server.ts`, gated on `PUBLIC_SENTRY_DSN` (+ `PUBLIC_SENTRY_ENVIRONMENT`).
- `sentrySvelteKit()` Vite plugin uploads source maps only when `SENTRY_AUTH_TOKEN`/`SENTRY_ORG`/`SENTRY_PROJECT` are set (build succeeds without them).
- No CSP change needed — no `default-src`/`connect-src`, browser delivery to ingest host is unrestricted.

## Config

`SENTRY_DSN`/`SENTRY_ENVIRONMENT` (backend + workers) and `PUBLIC_SENTRY_DSN`/`PUBLIC_SENTRY_ENVIRONMENT` (frontend), all optional, injected by `freehire-ops` (never committed). Two Sentry projects (frontend + backend); `SENTRY_ENVIRONMENT` tags events for shared project filtering.

## HTTP response metrics

`freehire_http_requests_total{method,status}` counts every API response, exported on the
`/metrics` listener. Two pieces, and both are needed:

- **`HTTPMetrics()`** — mounted as the OUTERMOST middleware in `cmd/server`, before
  `recover.New`. Counts only requests that returned no error.
- **`CountErrors(handler.RenderError)`** — wraps the app's `ErrorHandler`. Counts the rest.

**Why it is split, and the trap it exists for.** Fiber renders a returned error in the
`ErrorHandler`, which runs AFTER the middleware chain has fully unwound. A middleware reading
`c.Response().StatusCode()` after `c.Next()` therefore sees 200 for every 404 and every 500 —
the exact responses the counter exists to see. A recovered panic behaves the same way: `recover`
turns it into an error and the status is chosen later. The wrapper asks the real error handler
what status it sent rather than re-deriving it, because `RenderError` resolves the status
through `codedError` and `classify`, and a second copy of that mapping is the failure this
codebase has already paid for once.

**The method label is mapped, never passed through.** `c.Method()` is backed by fasthttp's
request buffer, which is RECYCLED between requests, so a label value taken straight from it
mutates after the counter has stored it. On prod 2026-08-20 that produced a label reading `GETT`
and a `/metrics` endpoint answering 500 — the corrupted label sets collided with the intact ones,
so the whole endpoint failed rather than degrading. `methodLabel` returns package-level constants
that share nothing with the request. It also bounds the label: the method is CLIENT-SUPPLIED, so
an unbounded passthrough would let any caller mint series at will.

**No route label on THIS counter, deliberately.** The app registers ~700 routes; route x status x
method is tens of thousands of series on a single-target Prometheus. Widening it is guarded by
`TestHTTPMetricsLabelsAreBounded`, which fails if a path label creeps in. The route lives in a
separate metric instead — below.

## Per-route traffic

`freehire_http_route_requests_total{route}` counts every response by the route PATTERN it matched,
and by nothing else. It is the separate metric the paragraph above reserves: route alone is ~700
series, which is affordable; route joined by a status or a method is not, and
`TestRouteMetricLabelsAreBounded` fails if either arrives. The two metrics answer two questions —
"is the error rate rising" is `freehire_http_requests_total`, "where is the traffic going" is this
one. Counted from the same two places, for the same reason.

**The label is the pattern, never the path.** `/api/v1/jobs/:id`, not
`/api/v1/jobs/senior-go-engineer-42`. A pattern is a string the router built at registration, so
it is bounded by the app rather than by the caller, and it shares nothing with the recycled request
buffer — the two properties `methodLabel` has to construct by hand.

**Both are lost for an unmatched request, which is the one case `routeLabel` guards.** Fiber's
`Ctx.Route()` synthesises a `Route` when nothing matched, and its `Path` is `c.pathOriginal` — the
caller's raw path, aliasing the request buffer. Passing that through would let any caller mint a
series per request AND repeat the label corruption that answered `/metrics` with a 500 on
2026-08-20. The synthetic route is identifiable by an empty `Handlers` slice (a registered route
cannot have one; Fiber panics at registration), and those requests are counted as `unmatched`.
A 404 that still passed a middleware lands on `/` instead, because Fiber leaves `c.route` at the
last middleware that ran and `app.Use` registers at `/` — bounded and counted, just coarser.

Grafana reads it as a top-K, since a full ~700-series legend is unreadable:
`topk(15, sum by (route) (rate(freehire_http_route_requests_total[5m])))`.

**This does not replace the site-alert watchdog and does not overlap it.** `site-alert.sh` polls
a real endpoint every two minutes and pages to Telegram on two consecutive failures; it caught
the 2026-08-19 schema outage at 11:37 and paged at 11:39, which no scrape interval improves on.
What a poll cannot see is a RATE — 0.5% of requests failing while the poll keeps succeeding.
That is this counter's job, and the two answer different questions.

**Live on prod, scraped per colour.** `METRICS_PORT` is 9091 (blue) / 9092 (green) in
`/opt/freehire/env/api-{blue,green}.env`, firewalled to the scraper at both ufw and the Hetzner
firewall; `StartMetricsServer` is still a no-op without the port, so a local run serves nothing.
Prometheus scrapes BOTH colours unconditionally with a `color` label — the standby slot simply
reads `down`, which is how you tell which slot is cold. That is also why every dashboard query
aggregates (`sum by (...)`) rather than reading a raw series: a release flips which colour carries
the traffic. The scrape job, the firewall rules and the alert rules live in `freehire-ops`.
