## Why

`hh` (headhunter.ru) job postings are landing in the catalogue without a real
description. `internal/ingest/sources/hhru.go`'s `FetchNew` hydrates each new
posting's description from its vacancy detail page, and that detail fetch is
routed through `SOURCES_PROXY_URL` (`proxiedProviders` in `proxy.go`), on the
documented assumption that "hh.ru's detail pages 403 the direct datacenter IP".

That assumption is now stale. Live investigation on prod (2026-09-08) confirmed:
the currently configured proxy egress IP is redirected by hh.ru's DDoS-Guard edge
to an interactive image CAPTCHA (`/account/captcha`, `server: ddos-guard`) on
essentially every detail-page request — not a JS challenge a headless browser
could solve, an actual image CAPTCHA. A live production run (`freehire-ingest@hh`,
16:19 UTC) failed 40/40 detail fetches in real time this way; the day's logs show
2938/2938 failures. Each failure falls back to list-only per the adapter's existing
design (`hhru.go`'s `detail()` comment), so the posting is stored carrying only the
salary paragraph instead of the real body — confirmed via the public API
(`/api/v1/jobs/search?source=hh`): roughly 45-50% of sampled hh postings hold only
a `<p>Зарплата: ...</p>` stub.

Two working alternatives were spiked and measured on the exact URLs that are
failing in production right now:
- The direct datacenter IP serves hh.ru detail pages cleanly today (80/80 success
  at the adapter's own production pace, ~4 req/s sustained).
- Firecrawl (`internal/platform/firecrawl`, already configured with
  `FIRECRAWL_API_KEY` on prod for `bayt`/`gulftalent`) also fetches them cleanly,
  bypassing the CAPTCHA entirely (1/1 spike success, real `JobPosting` description
  content returned, ~2900 chars).

The user has decided the fix should move `hh`'s detail hydration onto the
Firecrawl hosted-fetch tier, since it does not depend on the reputation of any
proxy or datacenter IP freehire controls — unlike routing back onto the direct
IP (which is exactly the state that led to the proxy being added in the first
place, per the adapter's own history) or the current proxy (already burned).

## What Changes

- `hh`'s detail-page hydration (`hh.detail()` in `internal/ingest/sources/hhru.go`)
  moves from the proxied `HTMLGetter` to the Firecrawl hosted-fetch tier
  (`internal/platform/firecrawl`), following the existing `bayt`/`gulftalent`
  precedent in `firecrawltier.go`.
- `hh`'s LISTING crawl (`hh.ru/search/vacancy`, the `HH-Lux-InitialState` blob) is
  unaffected — it is not observed to be failing (postings, titles, companies,
  salaries land fine today) and stays on its current transport.
- `proxy.go`'s stale "hh.ru's detail pages 403 the direct datacenter IP" comment
  and `hh`'s membership in `proxiedProviders` are corrected to reflect the
  measured, current reality and the new transport split between listing and
  detail.
- `FIRECRAWL_MAX_PAGES_PER_RUN` (currently 25, shared across every
  firecrawl-tier provider) is far too low for hh's volume (~690 detail
  fetches/hour observed, still settling from the 7-day backlog since boards
  onboarded 2026-09-03) and needs raising — an operator/deploy concern, not a
  code default, per the existing pattern (see design.md for the sizing and cost
  math).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — this is a transport-tier swap behind the existing `hh` adapter contract:
same `Job` shape, same `ExternalID`/dedup identity, same seen/hydrate/list-only
fallback behavior. No spec currently tracks `hh` as its own capability, and
`source-ingest`'s existing requirement — "MAY perform per-posting detail
requests... to obtain the description" — is unchanged; this fixes the adapter's
compliance with it, not the requirement itself.)

## Impact

- `internal/ingest/sources/hhru.go` — `detail()` takes a `firecrawl`-backed
  getter instead of `s.http` for the detail fetch; `crawl()`/listing unchanged.
- `internal/ingest/sources/firecrawltier.go` — `hh` added alongside
  `bayt`/`gulftalent`.
- `internal/ingest/sources/proxy.go` — `hh` removed from `proxiedProviders`;
  stale comment corrected.
- `internal/ingest/sources/AGENTS.md`, `internal/platform/firecrawl/AGENTS.md` —
  doc updates reflecting `hh` joining the firecrawl tier and why.
- Deploy-side: `FIRECRAWL_MAX_PAGES_PER_RUN` needs raising on prod
  (`/opt/freehire/.env`), and the Firecrawl plan/credit budget needs headroom for
  the added volume — both are operator actions outside this repo, called out in
  design.md and tasks.md so they aren't missed at deploy time.
- No schema, API, or dependency changes. No effect on other adapters.
