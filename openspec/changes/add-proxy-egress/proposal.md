## Why

The production datacenter IP is now IP-reputation-blocklisted by some ATS anti-bot systems: eightfold returns HTTP 403 on every endpoint from the prod IP (89.167.94.146) while the same request from a residential IP returns 200. This has degraded eightfold to ~87% of boards failing `streaming board failed with no progress`, and the same failure class already blocks 2gis, EPAM, and wantapply. No adapter change can fix an IP-level block — the crawl needs a different egress IP for these specific providers. The HTTP client (`internal/sources/fingerprinthttp.go`, `bogdanfinn/tls-client`) can already take a proxy URL, but nothing wires one in.

## What Changes

- Add optional proxy-egress support to the sources HTTP client: when a proxy URL is configured, requests can be routed through it instead of the direct datacenter IP.
- Make proxy egress **opt-in per provider** — only providers explicitly marked as IP-blocked route through the proxy; everything else keeps using the direct IP (proxy bandwidth is metered/paid, and most sources work fine directly).
- Add proxy configuration (URL/credentials) via env, consistent with existing worker config; absent config is a no-op (all providers stay direct).
- Onboard `eightfold` as the first proxied provider; leave 2gis / EPAM / wantapply as follow-on opt-ins once validated.

## Capabilities

### New Capabilities
- `source-proxy-egress`: opt-in per-provider routing of ingest HTTP requests through a configured egress proxy, so IP-blocklisted ATS sources can be crawled from a non-datacenter IP while all other sources stay on the direct connection.

### Modified Capabilities
<!-- None: proxy routing is additive; existing source-ingest behavior is unchanged when no proxy is configured. -->

## Impact

- **Code:** `internal/sources/fingerprinthttp.go` (and/or the sources HTTP client construction) to accept an optional proxy; the provider registry / adapter wiring to mark which providers are proxied; `internal/config` for the proxy env var(s).
- **Config/ops:** a new env var for the proxy URL+credentials on the ingest worker; a residential/rotating proxy provider account (external dependency, paid).
- **Sources:** `eightfold` opted into proxy egress; recovery expected once routed off the blocked IP.
- **No breaking change:** with no proxy configured, behavior is identical to today.
