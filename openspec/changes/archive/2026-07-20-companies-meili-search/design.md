## Context

`GET /api/v1/companies` (handler `internal/handler/companies.go:94` → `queries.ListCompanies`)
serves every company search/typeahead surface in the product. Today it filters with
Postgres `ILIKE '%q%'` on `name`/`slug` and orders by `job_count DESC, name` — no
relevance ranking, no typo tolerance, no prefix bias. An exact-name match with few
jobs is buried under higher-volume substring matches (the "arb" bug).

The codebase already runs Meilisearch for jobs (`internal/search`, `cmd/reindex`),
using a two-index atomic swap-rebuild (`jobs` / `jobs_rebuild`) and best-effort
incremental pushes from the ingest crawler keyed on `content_hash`. The `search`
package is hardwired to `JobDocument` and the `jobs*` index UIDs — there is no
generic index abstraction.

All company reads were inventoried: every search/typeahead/ranked-list consumer
funnels through `listCompanies` → `GET /api/v1/companies` (catalog SSR + client,
job-filter `company_slug` typeahead, referral `CompanyPicker`, global
`HeaderSearch`, `meta.total` counts). Point lookups (`GetCompany` by slug), sitemap,
writes, `RefreshCompanyFacets`, and the `insights` leaderboard do not search.

## Goals / Non-Goals

**Goals:**
- Relevance-ranked, typo-tolerant, prefix-aware company search with an exact-name
  match first and `job_count` as the tiebreaker.
- Cover every company-search surface by migrating the single `/companies` endpoint —
  zero frontend changes.
- Zero regression risk to the working jobs search: do not edit jobs code.
- No new failure point for `/companies`: Postgres path stays as fallback.

**Non-Goals:**
- Real-time (per-write) index freshness for companies — eventual consistency via a
  scheduled rebuild is sufficient (companies change slowly).
- Moving point lookups, sitemap, writes, `insights`, or `GET /companies/subindustries`
  off Postgres.
- Generalizing `internal/search` into an index-agnostic abstraction.

## Decisions

### Decision: Separate `CompanyIndex` parallel to jobs, not a generic refactor

Add `internal/search/company.go` with its own `CompanyDocument`, `FromCompany`
mapper, `companySettings()`, `SearchCompanies()`, and `RebuildCompanies()` methods
on `*Client`, plus its own UID constants (`companies`, `companies_rebuild`). The
existing jobs client (`facet`/`semantic` fields, `JobDocument`, `jobs*` UIDs) is
left byte-for-byte unchanged.

- **Why:** the jobs search is on the critical path and already has a history of
  Meili incidents (corruption, disk-full during reindex). A parallel type means the
  jobs path physically cannot regress from this change. Chosen by the user over a
  generic abstraction.
- **Alternative — generalize `Rebuild`/`Client` over a document interface:** cleaner
  long-term, but touches the jobs code path (regression risk) for a second consumer
  that is small and slow-moving. Deferred; the duplication is modest and honest.

### Decision: Meili ranking rules = default + `job_count:desc` tiebreaker

`companySettings()` uses Meili's default ranking (`words, typo, proximity,
attribute, exactness`) with `job_count:desc` appended as the final tiebreaker.
Searchable attributes ordered `name`, `slug`, `tagline` (attribute-rank favors
`name`). `exactness` gives the exact-name-first behavior the spec requires; the
appended `job_count:desc` reproduces today's secondary ordering among ties.

- **Why:** matches the spec's exact→prefix→contains + `job_count` tiebreaker with
  built-in behavior — no hand-rolled scoring.
- **Alternative — a hand-tuned Postgres `ORDER BY` relevance expression:** solves
  the "arb" case without new infra, but no typo tolerance and no prefix search, and
  the user explicitly wants the Meili path for all surfaces.

### Decision: Scheduled full swap-rebuild, no per-write outbox

`cmd/reindex-companies` keyset-scans `companies WHERE job_count > 0`, maps via
`FromCompany`, pushes into `companies_rebuild`, awaits, and atomically swaps to
`companies` (mirrors `cmd/reindex`'s `Prepare`/`Push`/`Promote`). Runs on cron with
its own flock, never stacked with the jobs reindex.

- **Why:** companies are a slow-moving directory (~hundreds of thousands of rows,
  changed by periodic backfills/recompute). Eventual consistency within the rebuild
  interval is acceptable; a per-write outbox (as jobs have) is unjustified plumbing.
- **Alternative — hook company pushes into every company-writing command:** matches
  jobs' freshness but adds `content_hash` tracking + push calls to `SyncCompanies`,
  `import-yc`, backfills, `RefreshCompanyFacets`. Not worth it here.

### Decision: Handler reroutes with Postgres fallback

`ListCompanies` uses Meili when search is enabled (`client != nil`) **and** a `q`
or facet filter is present; otherwise, or on any Meili error, it serves the current
Postgres path. The response shape (`{data, meta}`), the `job_count > 0` scope, and
the empty-`q` catalog ordering are preserved so the frontend and contract are
untouched. `meta.total` comes from Meili's `estimatedTotalHits` on the Meili path,
from `CountCompanies` on the Postgres path.

- **Why:** `/companies` currently works with no Meili dependency; keeping the
  fallback means the worst case (Meili down) silently degrades to today's behavior
  instead of a new outage.
- **Alternative — hard Meili cutover:** simpler handler, but makes the whole company
  catalog + every typeahead depend on Meili uptime. Rejected.

## Risks / Trade-offs

- **Facet-value fidelity between Postgres overlap and Meili filters** → Meili filter
  expressions must reproduce array-overlap (OR within facet, AND across) and scalar
  membership exactly, including the `NULL`-matches-none rule for `maturity`/
  `subindustry`. Covered by porting the existing facet scenarios as tests against the
  Meili path.
- **Eventual-consistency lag** → a brand-new hiring company or a just-recomputed
  facet is invisible to search until the next company reindex, and a company whose
  job_count has since dropped to 0 stays searchable (the index is scoped at build
  time, unlike the Postgres `WHERE job_count > 0`). Acceptable per the spec; the cron
  interval bounds the lag.
- **Cold-index window** → the handler falls back to Postgres only on a Meili *error*,
  not on an empty result (an empty result is a legitimate "no matches" that must not
  be second-guessed). So during the first-ever build — between `Prepare` (which creates
  the live `companies` index empty) and the first `Promote` swap, or before
  reindex-companies has ever run — a search hits an empty index and returns an empty
  list rather than falling back. This is the same cold-start behavior the jobs index
  has; the migration plan closes the window by running reindex-companies once at/before
  deploy so the empty index is never the live-serving one for long.
- **Shared Meili host disk during swap** → the rebuild transiently holds old+new
  index (~2× that index's disk). The companies index is small, but the reindex must
  not stack with the jobs reindex (own flock) to avoid compounding disk pressure —
  the same discipline already applied to jobs.
- **`meta.total` semantics drift** → Meili returns an *estimate* (`estimatedTotalHits`)
  where Postgres `CountCompanies` is exact. For a typeahead/catalog this is
  acceptable and matches how jobs search already reports totals.

## Migration Plan

1. Ship `internal/search/company.go` + `cmd/reindex-companies` + handler reroute
   behind the existing search-enabled gate (no Meili configured → pure Postgres,
   behavior identical to today).
2. Deploy; run `reindex-companies` once to build the `companies` index.
3. Add the cron entry (own flock, off-peak, not overlapping jobs reindex).
4. Verify ranked search on prod (`?q=arb` returns the exact match first; typo query
   resolves). Rollback = stop routing to Meili (the fallback path is the old
   behavior) or drop the `companies` index; jobs search is untouched throughout.

## Open Questions

- Cron cadence for `reindex-companies` (daily vs a few times a day) — decide with
  ops based on how fresh the directory needs to be; defaults to daily.
