# API latency notes (task 4.1)

Profiling of the two calls the company page makes, to pin the ~1s+ blocking wait
the streaming change works around. **Optimization is deferred** — this documents
where the time goes and proposes next steps.

## Method + caveat

Measured with `curl` from a developer laptop over the public internet to
`https://freehire.dev`. These numbers therefore include TLS/TCP setup (~0.5s
one-time) and real network RTT + body download — they are an **upper bound**, not
server-only time. In production the SvelteKit SSR node server calls the Go API
over the internal network (`API_INTERNAL_URL`), so its per-call latency is much
lower than what a laptop sees. True server-only timing needs a measurement from
the prod host (curl app→API, or a server-timing log), which wasn't available in
this session.

## Observations

| Endpoint | Body size | TTFB (ext) | Total (ext) |
|---|---|---|---|
| `GET /companies/stripe?limit=1` (PG) | 7 KB | 0.85–1.3s | ~1.0–1.6s |
| `GET /companies/stripe` (default limit=20, PG) | **140 KB** | ~0.8s | ~1.5s |
| `GET /jobs/search?company_slug=stripe&limit=20` (Meili) | 40 KB | ~0.4–0.8s | ~1.3s |

Clean single breakdown of `getCompany` (fresh connection):
`dns 0.004 · connect 0.25 · tls 0.50 · ttfb 0.87 · total 1.64` — i.e. ~0.5s is
connection setup (amortized under keepalive), leaving ~0.37s server+network TTFB
and ~0.77s body transfer.

## Findings

1. **The blocking wait was structural, not purely backend.** The old load
   `await`ed `Promise.all([getCompany, searchJobs])`, so the client navigation
   was held for the slower call (Meili search) plus the browser→SvelteKit hop,
   with zero visual feedback. The streaming split + nav indicator (this change)
   removes that regardless of backend speed.
2. **`getCompany` default response is heavy (140 KB):** it serializes 20 full job
   descriptions. The company-page load only needs the entity; it already requests
   `limit=1` (7 KB). The `limit=0` cleanup is a no-op because `pageParams` clamps
   `limit` to `>= 1` (`internal/handler/handler.go:94`).
3. **Meili search TTFB (~0.4s+) is the real slow call** on the company page and is
   the part now streamed behind the skeleton.

## Proposed (deferred) optimizations

- **Company-entity-only fetch.** Add a lean path that returns just the company row
  (no jobs) — either a dedicated handler or relaxing the `pageParams` clamp to
  allow `limit=0` (touches every list endpoint; weigh carefully). Removes the last
  discarded job from the company-page load and shrinks the default endpoint for
  other callers.
- **Measure server-only latency from the prod host** to separate network from PG
  vs Meili, before optimizing blind.
- **Investigate the Meili `company_slug` query cost** (filter + sort + any facet
  distribution) — the ~0.4s TTFB is the largest server-side component on this page.

These are tracked here as follow-ups; none block the frontend streaming change.
