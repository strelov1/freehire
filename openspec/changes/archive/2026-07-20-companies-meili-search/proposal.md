## Why

A company literally named "arb" does not rank first for `GET /api/v1/companies?q=arb`.
The endpoint filters with a Postgres `ILIKE '%arb%'` substring (on `name` and
`slug`) and orders purely by `job_count DESC, name` — there is **no relevance
ranking**, so an exact-name match with few open jobs is buried below every
higher-volume company whose name or slug merely contains the query. Every company
search surface in the product (catalog, three typeaheads, header search) is served
by this one endpoint, so they all inherit the same poor ranking and lack typo
tolerance and prefix matching.

## What Changes

- Add a **separate Meilisearch `companies` index** (parallel to the existing
  `jobs` index) built from the `companies` table, carrying the searchable text
  (`name`, `slug`, `tagline`), the denormalized `job_count`, and the existing
  facet arrays/scalars as filterable attributes.
- Route company search behind `GET /api/v1/companies` through Meili when search is
  enabled and a `q` or facet filter is present — giving relevance ranking (exact →
  prefix → contains), typo tolerance, and `job_count` as the tiebreaker — so an
  exact-name match surfaces first.
- Keep a **Postgres ILIKE fallback**: on any Meili error or when search is
  disabled/unavailable, the handler serves the current Postgres path, so
  `/companies` gains no new failure point.
- Add `cmd/reindex-companies`: a full swap-rebuild worker (reusing the jobs
  index's atomic `swap-indexes` pattern) over `companies WHERE job_count > 0`,
  scheduled on cron and never stacked with the jobs reindex. No per-write outbox —
  companies change slowly, eventual consistency is sufficient.
- **All company-search consumers are covered by this single endpoint** — catalog
  page, job-filter company typeahead, referral `CompanyPicker`, global
  `HeaderSearch`, and the filtered `meta.total` — with no frontend changes.
- Leave `internal/search`'s jobs code untouched (a parallel `CompanyIndex`, not a
  refactor of the jobs client), so the jobs search cannot regress.

Non-goals (kept on Postgres): point lookups by slug (`GET /companies/:slug`, OG,
match-analysis), the sitemap, all writes/maintenance (`RefreshCompanyFacets` etc.),
the `insights` leaderboard, and the `GET /companies/subindustries` facet vocabulary.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `companies`: the "Company list is served without joining jobs" requirement
  changes how the `q` search matches and ranks — from a Postgres `ILIKE` substring
  ordered by `job_count DESC, name` to a Meilisearch-backed ranked search (exact →
  prefix → contains, typo-tolerant, `job_count` tiebreaker) with a Postgres
  fallback. The response contract, the `job_count > 0` hiring scope, the facet
  semantics, and the empty-`q` catalog ordering are preserved.

## Impact

- **Code:** new `internal/search/company.go` (`CompanyDocument`, `FromCompany`,
  `companySettings`, `SearchCompanies`, `RebuildCompanies`); new
  `cmd/reindex-companies/main.go`; modified `internal/handler/companies.go`
  (`ListCompanies` reroute + fallback).
- **Infra/ops:** a new Meili `companies` index on the existing Meili host; a new
  cron entry for `reindex-companies` (own flock, not stacked with jobs reindex).
- **Config:** none new — reuses `MEILI_URL` / `MEILI_MASTER_KEY`.
- **Frontend:** none — the `GET /api/v1/companies` response contract is unchanged.
- **Dependencies:** none new — reuses the `meilisearch` Go client already vendored.
