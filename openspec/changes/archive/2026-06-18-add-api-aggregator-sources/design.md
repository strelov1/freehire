## Context

Five public job boards were verified live this session to expose clean JSON APIs with their
own inventory. They all fit the established **aggregator** adapter shape — boardless, one
global feed, company-per-posting — already used by `jobicy`, `remoteok`, and `tecla`. No new
abstraction is needed; this change is five instances of an existing pattern plus wiring.

## Goals / Non-Goals

**Goals:** add `workingnomads`, `himalayas`, `landingjobs`, `remotive`, `justjoin` as
boardless aggregator sources, each with a unit test over a captured fixture, registered in
`sources.All` and surfaced in the source facet.

**Non-Goals:** salary capture (no field on `sources.Job`); skills/seniority/geo parsing
(owned by the ingest dictionaries, applied downstream); cron scheduling (lives in
`freehire-ops`); any schema/migration/API change; a headless tier (this repo has none).

## Decisions

- **One change, five adapters.** They are homogeneous and share the pattern, so a single
  change/PR (one image rebuild, one deploy) is simpler than five near-identical PRs. Each
  adapter is still an independent task with its own RED→GREEN→review cycle.
- **`ExternalID` source per provider.** Use the platform's stable id where present
  (`himalayas.guid`, `remotive.id`, `landingjobs.id`, `justjoin.guid`). Working Nomads has
  **no** id field, so parse it from the `/job/go/<id>/` URL path; drop a posting whose URL
  yields no id rather than persist an empty id (which would collide under the
  `(source, external_id)` dedup key).
- **Canonical `URL`.** Use the feed's link where given. JustJoin's list response omits the
  apply link, so synthesize `https://justjoin.it/job-offer/<slug>` (the `jobstash` precedent
  for a protected/absent URL).
- **`WorkMode` from structured fields only.** `landingjobs.remote` (bool) →
  `workModeFromRemote`; `justjoin.workplaceType` → `workplaceTypeMode`. Remotive is
  remote-only → `"remote"` for all. Working Nomads and Himalayas expose only free-text
  location → leave `WorkMode` empty and let the ingest location dictionary resolve it
  (provenance stays clean: structured signal only, never the location heuristic).
- **Pagination, bounded.** Himalayas (offset/limit over `totalCount`) and JustJoin (follow
  `meta.next.cursor`) paginate under a defensive max-page cap (the `yandex` runaway-cursor
  incident: a tarpitted host must not loop forever). Remotive is fetched **once** — its API
  is rate-limited (~4 req/day, 24h delay), so a pagination loop is both unnecessary and
  abusive. Working Nomads returns its window in a single array.
- **Description sanitization.** All feeds carry HTML bodies inline → route through the shared
  `sanitizeHTML` (which also folds the nbsp-overflow characters per the existing fix).

## Rejected alternatives (from the same spike)

- **landing.jobs** — passed the spike (clean public `/api/v1/jobs` array, rich salary/location)
  but **dropped during implementation**: the list response carries **no structured company
  field**. The employer appears only as a URL slug (`/at/cliftonlarsonallen/…`) and inside the
  description prose. An aggregator's whole contract is a clean per-posting company → a clean
  `company_slug`; deriving it from a concatenated slug (`cliftonlarsonallen`) would pollute the
  companies table with malformed names — exactly the MVP-shortcut the project forbids. Re-add
  only if a structured company field is found (e.g. a detail/company endpoint) and the
  per-job cost is justified.
- **aidevboard** — clean API, but `apply_url` points at Greenhouse/Ashby boards we already
  crawl directly. Ingesting it as its own source would duplicate those jobs under a second
  `(source, external_id)`. It is only worth adding if resolved through `linksource` to dedup
  against the canonical ATS posting — deferred as a separate, harder change.
- **gitjobs.dev, WorksHub (`*.works-hub.com`)** — no public JSON API path discovered without
  a browser/repo dig; deferred.
- **nofluffjobs** — Cloudflare-fronted (fetch blocked); needs a headless tier this repo lacks.
- **jobscollider → remotefirstjobs** — `/api/v1/jobs` returns 401 (key-gated) and it
  re-aggregates ATS we already have; skipped.
- **devitjobs.com** — parked, redirects to a jobcopilot signup; dead.
- **Welcome to the Jungle** — job search behind an Algolia/private key; skipped.
- **Wellfound / Built In / HiringCafe** — auth/antibot and heavy re-aggregation of our own
  ATS (dup risk); skipped.
- **LinkedIn / Indeed / Glassdoor / Adzuna / Careerjet** — ToS / partner-key; out.

## Risks

- **Cross-source duplication** is the main risk for any aggregator. Mitigated by source
  selection: the five chosen feeds carry their own inventory (their `URL` points at
  themselves, or — landing.jobs/justjoin — at EU/PL companies under-represented in our ATS
  coverage), so overlap with our 28 ATS adapters is low. Residual dupes are the existing,
  accepted behavior (dedup is per `(source, external_id)`; the same vacancy from two sources
  is two rows by design).
- **API drift / rate-limits.** Each adapter fails its own board independently (the per-board
  crawl isolation already in the pipeline), so one feed breaking does not abort a run.
  Remotive's documented limit is respected by the single-fetch design.
