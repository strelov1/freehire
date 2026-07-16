## Context

The prod datacenter IP is IP-reputation-blocklisted by eightfold's anti-bot (verified 2026-07-16: prod IP → 403 on every eightfold endpoint; residential IP → 200; plain `curl` with no special headers works from residential, so the block is IP-based, not TLS-fingerprint or header based). ~87% of eightfold boards fail `streaming board failed with no progress`. The same class already blocks 2gis, EPAM, wantapply.

Two HTTP transports exist in `internal/sources`:
- The **standard client** — `safehttp.NewClient` → `safehttp.NewTransport` (net/http `*http.Transport` with an SSRF-guarded `DialContext`). Exposed to adapters as `JSONGetter`/`Client`. **eightfold uses this** (not the fingerprint client).
- The **fingerprint client** — `fingerprintHTTP` (bogdanfinn/tls-client, Chrome profile), used only by Meta/Bayt/GulfTalent for edges that fingerprint the TLS+HTTP/2 layer.

Since eightfold's block is IP-based and it rides the standard client, proxy support must land in the standard client. tls-client also supports a proxy (`WithProxyUrl`) for future fingerprint-based proxied sources, but that is out of scope here.

## Goals / Non-Goals

**Goals:**
- Route a curated allowlist of IP-blocked providers (starting with eightfold) through a configured egress proxy.
- Keep every other provider on the direct datacenter IP (proxy bandwidth is metered/paid).
- No behavior change when no proxy is configured.
- Keep proxy credentials out of logs and board-health `last_error`.

**Non-Goals:**
- Proxying the fingerprint client (Meta/Bayt/GulfTalent) — separate follow-on if their edges start IP-blocking.
- Proxying liveness/linksource (they probe arbitrary user-supplied URLs; keep them on the SSRF-guarded direct client — see Risks).
- Rotating-proxy pool management logic in-app; a rotating proxy is handled by the provider behind a single URL.
- Auto-detecting which providers are blocked; the allowlist is explicit.

## Decisions

**1. Proxy lives in the standard transport, selected per-provider via two clients.**
Build two `Client` instances at ingest wiring time: the existing direct client, and a proxied client (`safehttp.NewTransport` with `Transport.Proxy = http.ProxyURL(u)`). The provider registry marks which providers are proxied (a small `proxiedProviders` set keyed by `Provider()` name); the wiring hands the proxied client only to those adapters. *Alternative considered:* a per-request proxy toggle threaded through `JSONGetter` — rejected as it touches every adapter signature; a second client selected at construction is a smaller, contained seam.

**2. Config via a single env var `SOURCES_PROXY_URL`** (form `http://user:pass@host:port`), parsed in `internal/config`. Empty → no proxied client is built and the proxied-provider set is treated as direct (no-op). A set-but-unparseable URL is a fatal construction error (spec: fail-fast, no silent direct fallback for a provider you meant to proxy). *Alternative:* separate host/user/pass vars — rejected; a single URL matches how `http.ProxyURL` and proxy vendors express credentials.

**3. SSRF guard is retained on the dialer but its target-IP check no longer covers proxied targets.** With a proxy, `DialContext` connects to the *proxy* host; the proxy resolves the final target. The guarded Control hook still runs on the proxy connection (proxy must be a public IP), but it cannot vet the ultimate destination. This is acceptable because proxied providers are a curated allowlist of known public ATS hosts, not user-supplied URLs. Liveness/linksource stay on the direct guarded client. Keep the existing `TestFingerprintHTTPBlocksInternalTarget`-style contract test for the direct path.

**4. Eightfold is the only provider opted in for this change.** 2gis/EPAM/wantapply are follow-on additions to `proxiedProviders` once eightfold is validated in prod.

## Risks / Trade-offs

- **[SSRF surface on the proxied path]** → Restrict proxy egress to the curated allowlist of ATS providers with fixed public hosts; never route liveness/linksource (arbitrary URLs) through it. Document that adding a provider to `proxiedProviders` asserts its hosts are trusted.
- **[Proxy cost / bandwidth]** → Opt-in per provider keeps only blocked sources on the paid proxy; eightfold is a bounded set of ~54 boards.
- **[Credential leakage in errors]** → eightfold errors embed the request URL (target host), not the proxy URL, so the proxy creds are not in `last_error` today; add a test asserting the proxy userinfo never appears in a wrapped error, and never log the raw `SOURCES_PROXY_URL`.
- **[Proxy becomes a single point of failure]** → If the proxy is down, proxied providers fail their run and back off via board_health exactly as today; other providers are unaffected. No worse than the current all-403 state.
- **[Residential proxy also gets blocked over time]** → Rotating proxy behind the single URL mitigates; if it recurs, revisit pool rotation. Out of scope now.

## Migration Plan

1. Ship the code with `SOURCES_PROXY_URL` unset → complete no-op, safe to deploy.
2. Provision a residential/rotating proxy account; set `SOURCES_PROXY_URL` on the ingest worker env only.
3. Trigger `freehire-ingest@eightfold` manually; confirm boards recover (200s, `consecutive_failures` → 0) via the ingest monitor.
4. Rollback: unset `SOURCES_PROXY_URL` and restart the ingest worker → back to direct (blocked) egress, no code revert needed.

## Open Questions

- Which proxy vendor/plan (residential vs. rotating datacenter)? Residential is confirmed to pass eightfold; datacenter may just re-trigger the block.
- Should the proxied-provider allowlist live in code (`proxiedProviders` set) or be env-configurable (`SOURCES_PROXY_PROVIDERS=eightfold,2gis`)? Code-set is simpler for one provider; env-list avoids a deploy to add the next. Leaning env-list for operational flexibility — decide in tasks.
