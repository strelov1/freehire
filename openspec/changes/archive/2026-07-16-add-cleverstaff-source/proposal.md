## Why

CleverStaff (cleverstaff.net) is a recruiting-ATS SaaS whose customers — product companies
and staffing agencies across UA/CEE — publish their open roles on public per-tenant pages.
A spike VALIDATED that every tenant's open vacancies are served as a keyless JSON payload
carrying the full HTML description, stable ids, employer name, and public apply link on a
single request. Adding one adapter widens first-party coverage of that market at the cost of
a new source plus a curated tenant list.

## What Changes

- Add a `cleverstaff` source adapter that fetches one tenant's open vacancies from
  `https://cleverstaff.net/hr/public/getAllOpenVacancy?alias=<board>` and maps each object to
  a normalized `Job` (title from `position`, sanitized `descr` description, `vacancyId` →
  `ExternalID`, public URL `https://cleverstaff.net/i/vacancy-<localId>`, `dc`/`dm` epoch-ms →
  `PostedAt`, structured `workCondition` → work mode and `employmentType` → employment type).
- The adapter is a **per-tenant ATS** keyed by `board` (the tenant `alias`), like
  greenhouse/lever — **not** boardless and **not** an aggregator: a CleverStaff posting is a
  first-party vacancy and must NOT be reindex-suppressed against ATS twins.
- Honor the existing `CompanyEntry.Hub` marker: a tenant flagged `hub: true` (a staffing
  agency) resolves each vacancy's employer from the feed's `clientName`, falling back to the
  configured company; an ordinary tenant keeps its configured company. No new hub mechanism —
  it reuses the seam huntflow's `AlumniHub` already uses.
- Add `sources/cleverstaff.yml` seeded with tenant aliases discovered via search-engine
  dorking and validated against the API (a run-once, out-of-band harvest — not code).
- Enroll `cleverstaff` in `sources.All` (one line) and — mirroring djinni — in
  `proxiedProviders`, so if the prod datacenter IP is blocked the crawl can egress through
  `SOURCES_PROXY_URL` without further code change.

## Capabilities

### New Capabilities
- `cleverstaff-source`: the `cleverstaff` adapter — its per-tenant JSON fetch, object→`Job`
  mapping, the drop rules for unusable objects, hub employer resolution, and its
  per-tenant-board classification.

### Modified Capabilities
<!-- None. Hub resolution reuses CompanyEntry.Hub unchanged; ATS-dedup does not apply
     (cleverstaff is first-party, not an aggregator). No existing spec's requirements change. -->

## Impact

- **New code:** `internal/sources/cleverstaff.go` (+ `_test.go`); `sources/cleverstaff.yml`.
- **Touched code:** one line in `sources.All`; one entry in `proxiedProviders`.
- **Ops:** a new `cmd/ingest sources/cleverstaff.yml` cron schedule (deploy-time, in
  freehire-ops). If the prod IP is blocked, the timer stays disabled until `SOURCES_PROXY_URL`
  is set — same rollout djinni used.
- **No migrations, no API changes, no new dependencies.**
