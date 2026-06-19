## Context

The sidebar Company filter (shipped in PR#202) is a `dynamic` Meilisearch facet,
the same machinery as `skills`/`countries`. That works for bounded vocabularies —
`skills` has 238 distinct values (under Meili's `maxValuesPerFacet=300` cap) and
`countries` 129 — so their distributions load fully and client-side search finds
everything. Companies are different: the catalogue has ~1.54M jobs across
thousands of companies. Meili returns only the first 300 facet values **in
alphabetical order**, so the company list is junk (`0-compromise-recruitment…`,
`01-tech`, counts of 1) and excludes popular employers — "google" finds nothing.

The companies endpoint (`GET /api/v1/companies?q=`) already exists and already
returns `{slug, name, job_count}` with a case-insensitive name search and
pagination. The `/companies` page already renders job-count badges, search,
debounce, and infinite scroll. So the data source and one consumer already exist;
they are just (a) name-ordered and (b) computing `job_count` via a query-time
`LEFT JOIN ... GROUP BY count()` over 1.54M jobs on every request.

## Goals / Non-Goals

**Goals:**
- A sidebar Company filter that searches the whole catalogue by name and lists
  companies most-active first, showing real names and job counts.
- A cheap, popularity-ordered `GET /api/v1/companies` reused by both the sidebar
  typeahead and the `/companies` page.
- Keep the company filter wired through the existing `company_slug` search param
  so URL sync, exclusion, chips, and active-filter-count are unchanged.

**Non-Goals:**
- Real-time exactness of `job_count` (eventual consistency within the recompute
  interval is acceptable).
- Per-filter *contextual* company counts (the count is global open-job count).
- Changing the `skills`/`countries` facets — they are within the cap and fine.
- Any Meili reindex (the document shape is unchanged).

## Decisions

### Denormalize `job_count`, maintained by a periodic recompute (not triggers)

Add `companies.job_count INT NOT NULL DEFAULT 0` plus an index on `(job_count
DESC)`. A new one-shot worker `cmd/recount-companies` runs a single set-based
`UPDATE` that sets every company's `job_count` from `GROUP BY company_slug` over
`jobs WHERE closed_at IS NULL` (companies with no open jobs → 0). Scheduled hourly
by host cron with `flock`, following the existing worker pattern
(`worker.Bootstrap`, run-once, exit codes), like `cmd/backfill-derive`.

- **Why over PG triggers:** the count changes both on ingest and on *close*
  (`closed_at` set by the ingest sweep and the liveness worker), across 1.54M
  rows crawled frequently. Triggers would add write overhead to every upsert and
  risk drift on any path that bypasses them. A periodic full recompute is simple,
  self-healing, and off the hot path. The user chose this explicitly.
- **Why over query-time count:** sorting all companies by an on-the-fly
  `GROUP BY` over 1.54M jobs on every `/companies` and every typeahead keystroke
  is the cost we are removing.

### Reuse the companies endpoint; change only ordering and the count source

`ListCompanies` drops the `LEFT JOIN`/`GROUP BY`/`count()`, reads `c.job_count`,
and orders `job_count DESC, name`. `q` ILIKE search and `CountCompanies` are
unchanged. `GetCompany` is `SELECT *`, so `job_count` flows into `db.Company`
automatically. No new endpoint — the sidebar typeahead and `/companies` share it.

### Frontend: a `control: 'remote'` facet type, state still keyed by `company_slug`

Add a new control type to the facet registry and a `RemoteSearchSelect.svelte`
that debounce-fetches `api.listCompanies(q, limit, 0)`, renders name + count,
shows the popular first page on an empty query, and on select calls the existing
`store.toggle('company_slug', slug)`. Because state stays keyed by the
`company_slug` param, `filtersToParams`/`filtersFromParams`, exclusion, chips, and
active-filter-count all keep working untouched — only the *option source* changes
from the Meili distribution to the endpoint.

- **Why a control type over a bespoke component:** it is the natural extension of
  the existing `pills`/`select`/`tokens` pattern and keeps the company filter
  inside the registry-driven system. The component takes a fetch function, so it
  is not company-hardcoded, but company is its only consumer for now (no further
  generalization until a second remote facet appears).
- **Selected-chip labels:** accumulate a session-local `slug → name` map from
  results so chips show real names; fall back to the existing `companyLabel(slug)`
  humanizer for URL-preselected slugs not yet seen (e.g. `?company_slug=stripe`).
  No batch slug→name endpoint is added (YAGNI).

### Stop requesting the unused `company_slug` facet distribution

`company_slug` stays in `search.StringFacets` (still needed for `?company_slug=`
filtering), but is excluded from the attributes the facets endpoint requests
distributions for (`facetAttributes()` in `internal/handler/facets.go`). We were
computing a 300-bucket distribution on every sidebar interaction that the UI no
longer reads.

## Risks / Trade-offs

- **Stale counts between recomputes** → hourly cron keeps drift small; counts are
  popularity hints and sort keys, not exact figures. Acceptable per Goals.
- **Recompute cost on 1.54M jobs** → it is a single aggregate + set `UPDATE`, not
  per-row; runs off the hot path under `flock` so runs can't stack.
- **Empty `job_count` immediately after deploy** → the column defaults to 0 until
  the first recompute, so the list would look empty/unsorted. Mitigation: run
  `cmd/recount-companies` once manually right after the migration, before relying
  on the UI (captured as a deploy task).
- **Global (non-contextual) company count** differs from other facets' contextual
  counts → intended and documented in the spec; the contextual path is the broken
  one being replaced.

## Migration Plan

1. Ship migration `0025_companies_job_count.sql` (column + index). On prod, apply
   it manually via `psql` (initdb runs only on first volume init).
2. Deploy a **full** Go image rebuild (new `cmd/recount-companies` binary) — not a
   sources-only deploy. Deploy the web image for the new facet control.
3. Run `cmd/recount-companies` once manually to populate `job_count`.
4. Add the hourly cron line (`flock` + `docker compose run --rm -T
   recount-companies`) and the compose worker service.
5. Verify: `/api/v1/companies` is count-ordered; sidebar typeahead finds "google";
   `/companies` page ordered by popularity.

Rollback: the change is additive (new column, new binary, new optional cron).
Reverting the web image restores the prior facet; the column can be left in place
harmlessly.

## Open Questions

None — scope and semantics were confirmed with the user during brainstorming.
