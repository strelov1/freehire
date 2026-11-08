## Why

Nine sources cannot be crawled today. The hosted Firecrawl tier is out of credits (`api status
402` on wantapply since 2026-10-18) and bayt / gulftalent have had no successful crawl since
2026-09-07; InfoJobs refuses the crawler host's IP and challenges the proxy after five pages;
the rest answer 403, time out, or fail TLS on our plain Go client. A spike on host2
(2026-10-20, `docs/superpowers/specs/2026-10-20-scrapling-fetch-design.md`) showed a
self-hosted Scrapling service fetches every one of them, at no per-page cost. (tbank, also failing, is a
TLS trust-store problem rather than an anti-bot one, and is out of scope.)

## What Changes

- New `services/scrapling-fetch`: a small HTTP service on host2 wrapping Scrapling's two
  fetchers — `http` (Chrome TLS impersonation) and `browser` (stealth Chromium that solves
  Cloudflare challenges) — with warm per-host browser sessions, a cap on live browsers, the
  proxy credential held service-side, and bearer-token auth on `127.0.0.1`.
- New `internal/platform/scraplingfetch`: an `http.RoundTripper` that forwards a request to
  the service, so an ordinary ingest `Client` (retries, body caps, status errors, every getter)
  runs over it unchanged.
- New `scraplingProviders` allowlist, applied at `cmd/ingest` startup, naming each provider's
  mode and proxy: infojobs, bayt, gulftalent, wantapply (sitemap), wellfound, hh (detail),
  startupjobs, cleverstaff, kontur. A no-op without `SCRAPLING_FETCH_URL`.
- InfoJobs listing spread across parallel browser sessions; its detail pages re-tested through
  the service, and hydration restored if they pass.
- **BREAKING (operator):** the hosted Firecrawl tier is removed — `internal/platform/firecrawl`,
  `firecrawltier.go`, and the `FIRECRAWL_API_KEY` / `FIRECRAWL_MAX_PAGES_PER_RUN` settings.

## Capabilities

### New Capabilities

- `scrapling-fetch-tier`: the self-hosted fetch service, its Go transport, and the per-provider
  allowlist that routes the sources no plain client can reach through it.

### Modified Capabilities

- `wellfound-source`: its pages are fetched through the Scrapling tier instead of the hosted
  Firecrawl tier.

## Impact

- Code: `internal/ingest/sources` (allowlist, adapters' transport wiring, removal of
  `firecrawltier.go`), `internal/platform/scraplingfetch` (new),
  `internal/platform/firecrawl` (removed), `cmd/ingest/main.go`,
  `internal/platform/arch/layering/blocks.go`, docs (`AGENTS.md` table, sources/firecrawl
  AGENTS files).
- New Python service and Dockerfile under `services/scrapling-fetch`; CI lint for both.
- Host: a container and systemd unit on host2 (in `freehire-ops`, deployed by hand); new env
  `SCRAPLING_FETCH_URL`, `SCRAPLING_FETCH_TOKEN`, `SCRAPLING_MAX_BROWSERS`.
- Load: browser fetches cost ~1.7 GB RAM per live session on host2 (16 cores, 25 GB free).
