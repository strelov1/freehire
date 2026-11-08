## Context

Three transports exist for hard sources: `proxiedProviders` (another IP), `browserProviders`
(our own chromedp Chrome, today only echojobs) and `firecrawlProviders` (a metered hosted API:
bayt, gulftalent, wellfound, wantapply's sitemap, hh's detail pages). The hosted API is out of
credits, and several more sources fail on the plain client. The spike that measured all of
this, per source and per mode, is in `docs/superpowers/specs/2026-10-20-scrapling-fetch-design.md`.

Crawlers run on host2 (`10.77.0.2`, 16 cores, ~25 GB free), deployed by `release.sh` from fsn.
Host configuration lives in the private `freehire-ops` repository.

## Goals / Non-Goals

**Goals:**

- One self-hosted fetch service on host2 serving both Scrapling fetchers.
- Adapters keep their code: the service is reached through the existing `Client` and getters.
- Every source the spike rescued is crawled through it; the hosted Firecrawl tier is gone.

**Non-Goals:**

- Moving the 200+ adapters a plain client serves.
- echojobs, neogov, dodo, energyjobline, jobleads, wuzzuf, workatastartup, naukri.
- Replacing the chromedp browser tier (its one provider, echojobs, is not rescued either way).
- A residential proxy pool.

## Decisions

**A thin service of our own, not Scrapling's MCP server.** The MCP server is built for agents
(open browser, click, screenshot) and defaults to markdown; adapters need raw bytes and status.
Our own `POST /fetch` returns exactly what the adapter would have read, is debuggable with
`curl`, and lets the service own session reuse. Alternative rejected: driving MCP sessions from
Go would put browser lifecycle in the crawler.

**Async server on the image's own starlette + uvicorn.** Both ship in the pinned image (MCP
dependencies), as do `AsyncStealthySession` and `AsyncFetcher`. Playwright is not thread-safe,
so a threaded stdlib server (the `pii-filter` shape) would need a lock per session; one event
loop avoids that. No new Python dependency.

**Image pinned by digest** (`ghcr.io/d4vinci/scrapling@sha256:1d876879813759959bc4ba6fbb1ce8247ebbc2b67eee77b140baf7c80af6ae6b`, 0.5): the service
reaches into Scrapling's session API, and a floating `latest` would change it without a diff.

**Go side is an `http.RoundTripper`, not a new getter type.** `sources.Client` wraps an
`*http.Client`; giving it a transport that forwards to the service keeps retries, the 64 MiB
body cap, `StatusError`, `BodyTooLargeError` and every getter, POST paths included, as the
existing code. The Firecrawl client re-implemented `GetXML`/`GetHTML`
and could do nothing else. Alternative rejected: a second getter implementation per interface.

**Package in `platform`** (`internal/platform/scraplingfetch`), beside `safehttp`: transport,
no domain knowledge. `sources` owns the allowlist and builds the `Client`.

**Proxy held by the service.** The crawler sends `proxy: true`; `SOURCES_PROXY_URL` is read only
by the service. No credential in request bodies or logs.

**Sessions keyed by `(host, proxy)`.** A Cloudflare clearance is per host and per exit IP;
keying that way solves a challenge once per crawl. Idle TTL closes a session; a global cap
(`SCRAPLING_MAX_BROWSERS`, default 3, ~1.7 GB each) bounds memory. A parallel crawl of one host
(InfoJobs) holds several sessions for that host: a request whose key's sessions are all busy
opens another, still under the global cap. A request queues for a session at most 120 s, below
the Go client's timeout, so one its caller abandoned does not later spend a browser.

**No client-side retries over the tier.** The service retries once; `sources.Client`'s own two
retries on top would turn one dead browser page into ~15 minutes.

**Internal targets refused in the service.** The plain client's `safehttp` guard does not see a
request the service makes, and bayt/gulftalent follow links scraped from the sources' pages, so
the service resolves the host and refuses loopback, private and link-local addresses itself.

**InfoJobs fetches its listing pages concurrently** (3 at once, the default browser cap): its
first page states the total, which fixes the page count; any failed page still fails the crawl.

**Modes per provider are the cheapest the spike saw pass**, re-verified live before each row
lands: `http` is ~0.2-2 s, `browser` ~6-9 s after the first page.

**Bound to `127.0.0.1` with a bearer token.** Crawlers run on the same host; nothing else needs
it. The token is defence in depth against other tenants of host2.

## Risks / Trade-offs

- [A source tightens its defence again] → the crawl fails loudly through board health, as
  today; the per-provider mode is one line to change.
- [Browser memory on a shared host] → global cap and idle TTL; container memory limit.
- [InfoJobs at ~7.5 s/page is ~6 h on one session] → 3 pages in flight (~2 h); cadence daily
  with a timeout sized to the measured run. If still too slow, its timeout grows rather
  than the crawl going partial (partial crawls already fail, #3336).
- [Service down stops nine providers at once] → systemd restarts it; board health shows it the
  same day. No silent fallback to the direct client, which would only be refused.
- [Scrapling is a single-maintainer project] → pinned digest; the Go side depends only on our
  own `/fetch` contract, so the engine behind it can be swapped.

## Migration Plan

1. Merge with `SCRAPLING_FETCH_URL` unset: nothing changes in production except that the
   Firecrawl tier (already failing on 402) is gone.
2. Build the image and start the unit on host2 by hand (`freehire-ops`); smoke `/health`.
3. Set `SCRAPLING_FETCH_URL` / `SCRAPLING_FETCH_TOKEN` in host2's `.env`; next crawls pick it up.
4. Re-enable InfoJobs' schedule window as measured; confirm each provider's next run in
   `board_health`.

Rollback: unset `SCRAPLING_FETCH_URL` — every provider returns to its plain transport.

## Open Questions

- Whether InfoJobs detail pages pass through browser + proxy (decides whether hydration
  returns). Answered by a task, not a guess.
