## Why

A spike of the public job-board ecosystem (the `awesome-job-boards` family of lists)
surfaced multi-company job boards that each expose a **clean, public JSON API** and carry
their **own** inventory — i.e. their postings are not just re-listings of the
Greenhouse/Lever/Ashby boards we already crawl, so they add net-new vacancies with a low
cross-source duplication risk. They all fit the existing **aggregator** adapter shape
(`jobicy`, `remoteok`, `tecla`): one boardless feed, company taken per posting. Several
neighbouring boards were rejected in the same spike (key-gated, parked, or Algolia/Cloudflare
behind a headless wall — which this repo has no tier for), and `landing.jobs` was dropped
during implementation when its list API turned out to carry no structured company field (see
design). The four kept here are both additive and reachable without a browser.

## What Changes

- Add four `boardless` + `aggregator` source adapters, each over a verified public JSON API,
  each mapping a posting to `sources.Job` with the employer taken from the posting:
  - **`workingnomads`** — `GET https://www.workingnomads.com/api/exposed_jobs/` (flat JSON
    array; no `id` field → `ExternalID` parsed from the `/job/go/<id>/` URL path).
  - **`himalayas`** — `GET https://himalayas.app/jobs/api` (offset/limit pagination over
    `totalCount`, ~90k jobs; bounded by a defensive max-page cap).
  - **`remotive`** — `GET https://remotive.com/api/remote-jobs` (single fetch only — the API
    is rate-limited to ~4 requests/day with a 24h data delay, so no pagination loop).
  - **`justjoin`** — `GET https://api.justjoin.it/v2/user-panel/offers/by-cursor` (PL/CEE IT;
    cursor pagination via `meta.next.cursor`; `workplaceType` → `WorkMode`; apply URL absent
    from the list response → canonical `URL` synthesized as `https://justjoin.it/job-offer/<slug>`).
- Register each `New…(c)` in `sources.All` and add a placeholder `sources/<provider>.yml`
  (one entry: `company` + `provider`, boardless so no `board`).
- The four providers join the source facet via `gen-contracts` (they are aggregators, so
  `FilterableProviders` keeps them) — `web/src/lib/generated/contracts.ts` SOURCE_VALUES is
  regenerated.
- Each adapter ships a table-driven unit test over a captured live fixture, matching the
  existing `internal/sources/*_test.go` pattern.

Out of scope (intentional): no salary/skills/seniority handling beyond what the ingest
dictionaries already derive (no salary field on `sources.Job`); the per-provider daily cron
lives in the separate `freehire-ops` repo (a follow-up, one schedule per provider). Rejected
sources from the same spike (recorded in design): `aidevboard` (apply_url is Greenhouse/Ashby
→ would duplicate our existing ATS jobs unless resolved through `linksource`), `gitjobs.dev`
and WorksHub (no public API path found), `nofluffjobs` (Cloudflare/headless), `jobscollider`
(now `remotefirstjobs`, 401 key-gated), `devitjobs` (parked → signup redirect),
Welcome-to-the-Jungle (Algolia key), Wellfound/BuiltIn/HiringCafe (auth/antibot + heavy
re-aggregation of our own ATS), and the LinkedIn/Indeed/Glassdoor giants (ToS).

## Capabilities

### New Capabilities
<!-- none — this extends the existing source-ingest capability -->

### Modified Capabilities
- `source-ingest`: registers four new **aggregator** (boardless, company-per-posting)
  providers — `workingnomads`, `himalayas`, `remotive`, `justjoin` — each crawling a single
  public JSON API. Reinforces the existing aggregator requirement (company derived from the
  posting, not the configured board) and adds the provider-specific constraints: `remotive`
  performs a single rate-limited fetch (no pagination), and `justjoin` synthesizes a canonical
  detail URL from the posting slug when the feed omits the apply link.

## Impact

- New: `internal/sources/{workingnomads,himalayas,remotive,justjoin}.go` + a `_test.go` for
  each, and `sources/{…}.yml` for each.
- Modified: `internal/sources/source.go` (four lines in `All`); regenerated
  `web/src/lib/generated/contracts.ts` (four new SOURCE_VALUES entries).
- No schema, migration, or HTTP-API change. New jobs flow through the existing
  `UpsertJob`/enrichment path unchanged. Reaching the catalogue in production is a
  `freehire-ops` cron addition (out of this repo) plus a `make reindex`.
