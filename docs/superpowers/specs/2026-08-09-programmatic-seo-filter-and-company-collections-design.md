# Programmatic SEO: cross-linked filter & company collections

**Date:** 2026-08-09
**Status:** approved, not yet planned/implemented

## Problem

`freehire` already has a job-facet-based programmatic SEO surface
(`/collections/[slug]`, backed by `FILTER_COLLECTIONS` in `web/src/lib/collections.ts`)
covering remote-region and skill landing pages. Three gaps limit its SEO value:

1. Its `<title>`/`<h1>`/OG tags are static strings — they never surface the live
   job count, so search snippets look generic and don't communicate freshness/scale.
2. The collection list is small and hand-picked; several obvious high-intent
   patterns (more countries, more roles, more skills) aren't covered.
3. There's no internal-link path from a job posting into these collection pages,
   so Google has to discover and re-crawl them independently rather than reaching
   them from the highest-volume page type on the site (job postings).
4. Company-level facets that already exist as filterable data — YC status
   (`yc_flags`, `yc_batch`), industry, company type — have no landing-page
   equivalent at all; they're only reachable via manual `/companies?...` filters.

## Non-goals

- No auto-generated cross-product of every facet combination. Combinations stay
  hand-curated (same discipline as today's `FILTER_COLLECTIONS`: each entry is
  added because it has a healthy non-empty live count and a plausible real
  search pattern), to avoid thin/duplicate-content pages.
- No new backend endpoint. Every count needed here is already returned by
  `searchJobs(..., limit=0)` / `searchCompanies` list totals or `facetCounts`.
- No keyword-volume-driven prioritization for the initial list — Search Console
  isn't connected yet for this project. The initial expanded list is judgment-based
  on standard job-board search patterns; it should be revisited once GSC data
  or GA4 data is available.

## Design

### 1. Live job count in `/collections/[slug]` title / H1 / OG

`collections/[slug]/+page.server.ts` already calls `serverApi(fetch).searchJobs(...)`
to SSR the first page of jobs, and that response's `meta.total` is the exact
same live count already available today (see `internal/handler` job list
response shape: `{"data": ..., "meta": {"total": ...}}`). No new fetch, no
caching — the number is already in hand at render time.

- `<title>`: `"{total.toLocaleString()} {collection.title} · freehire"`
  (e.g. `"1,234 React Jobs · freehire"`) — exact count, not rounded, per
  explicit product decision (accepting that Google's cached snippet may lag
  the live number between re-crawls, same tradeoff every job board with a
  live counter makes).
- `<h1>`: same count, e.g. `"1,234 React Jobs"` (today's H1 is just
  `"{collection.title} jobs"` — insert the count).
- OG title: mirrors `<title>`.
- Edge case: `total === 0`. This shouldn't occur for collections already in
  `FILTER_COLLECTIONS` (each is required to have a healthy non-empty count
  before shipping), so no special empty-state copy is needed — if it ever
  happens, it reads as "0 X Jobs," which is honest and not broken.

### 2. Expand `FILTER_COLLECTIONS`

Straightforward data addition to `web/src/lib/collections.ts`, following the
existing entry shape (`slug`, `title`, `description`, `params`) and existing
discipline (verify each new entry has a healthy live count via
`searchJobs(params, 0, 0).total` before adding it — do this as part of
implementation, not design). Candidate axes to grow, using only facet values
that already exist as first-class dictionary entries (`internal/skilltag`,
`internal/location` region/country codes, `internal/classify` role/category):

- More country-level remote landings beyond the current six regions (e.g.
  additional single-country remote pages where demand is plausible: Canada,
  UK, India, Poland — verify count at implementation time).
- More skill landings beyond the current set, for skilltag canonicals not yet
  covered.
- Role-level landings (e.g. `frontend-engineer`, `data-engineer`) alongside
  the existing hand-curated `role` entries, reusing `internal/classify`
  category/role vocabulary.

No structural change to the `FilterCollection` type or the route — this is
purely growing the array.

### 3. New company-collection page type

Mirrors the job-collection pattern one-to-one for company-level facets
(`yc_flags`, `yc_batch`, `company_type`, `subindustry` — all already
filterable on `GET /api/v1/companies`, per `internal/ycdir/AGENTS.md` and
`internal/handler/companies.go`).

- New data file, same shape as `FilterCollection`: `slug`, `title`,
  `description`, `params` (company-search facet params instead of job-search
  ones). Lives alongside `FILTER_COLLECTIONS` in `collections.ts` as e.g.
  `COMPANY_FILTER_COLLECTIONS`, or a sibling file if that keeps the module
  focused — implementation's call.
- New route `web/src/routes/companies/collections/[slug]/+page.svelte` +
  `+page.server.ts`, structurally mirroring `collections/[slug]`: resolve slug
  → params, SSR the first page of `/companies` filtered by those params, live
  count from the list response total.
- Title/H1/OG pattern matches part 1: `"{total} {title} · freehire"`, e.g.
  `"142 YC Companies Hiring · freehire"`.
- First entries: YC-backed (`yc_flags` overlap on `hiring` and/or `top_company`),
  possibly split by `yc_batch` era if counts justify it. Do not import YC
  `tier` as a facet (project convention, per `hire-backer-badges-shipped`:
  tier is not meant to be surfaced as a market-facing facet).

### 4. "See also" block on the job posting page

New component (or inline block) on `web/src/routes/jobs/[slug]` (the job
detail page) rendering a fixed-size set of internal links
(target ~4-6) built from two independent sources, evaluated at SSR time using
data the job page already has loaded (its own facets, and its company's
`yc_flags`):

- **Source A — job facets:** the current job's role/category, region
  (country/remote-region), and skills, matched against existing
  `FILTER_COLLECTIONS` entries (exact `params` match against the job's own
  facet values — e.g. job has `skills: ['react']` → matches the `react`
  collection entry).
- **Source B — company facts:** if the job's company has YC flags set, match
  against `COMPANY_FILTER_COLLECTIONS` (e.g. `yc_flags` includes `hiring` →
  link to the YC-hiring company collection).
- **Fill logic:** concatenate A then B matches (order: most specific first —
  skill/role before region, job facets before company facts), dedupe by slug,
  then if the combined count is below the target size, pad with a fixed
  fallback list of the most general/popular collections (e.g.
  `remote-worldwide`, top 1-2 skill collections) until the target is reached
  or the available collection pool is exhausted (never fabricate a link to a
  non-existent slug).
- Every link target is a real, already-existing `/collections/[slug]` or
  `/companies/collections/[slug]` page — this block never links to a slug
  that isn't in one of the two curated arrays.
- No live count fetch needed for the block itself (the linked pages compute
  their own count on load); the block only needs the two static arrays plus
  the current job/company's already-loaded facet values.

## Testing

- Unit test for the "see also" matching/padding logic (source A + source B +
  dedupe + fallback-fill + never-exceed-available-pool), independent of any
  HTTP call — pure function over (job facets, company facts, both collection
  arrays) → ordered slug list.
- Verify title/H1/OG count interpolation on both collection route types with
  a fixture total (e.g. snapshot the rendered `<title>` string given a known
  `meta.total`).
- Manual check per new `FILTER_COLLECTIONS`/`COMPANY_FILTER_COLLECTIONS` entry
  during implementation: live count is non-empty (same discipline as existing
  entries), per the design's non-goal on avoiding thin pages.
