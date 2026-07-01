## Context

The `/companies` catalog is a search-only list. A company row has `slug`, `name`,
`collections TEXT[]`, and a denormalized `job_count` (maintained by the periodic
`cmd/recount-companies` worker via `RecountCompanyJobCounts`). All richer facts —
geography, industry, type, size — live on the company's **jobs**, not the company:

- `jobs.regions` / `jobs.countries` are top-level `TEXT[]` columns, populated
  deterministically at ingest by `internal/location`.
- `enrichment.domains` (array), `enrichment.company_type`, `enrichment.company_size`
  live only inside the `jobs.enrichment` JSONB, populated asynchronously by the
  LLM enrichment worker.

The jobs page already has a mature filter sidebar: a URL-synced `FilterStore`
(`web/src/lib/filters.ts`), a data-driven `FACETS` registry (`web/src/lib/facets.ts`),
and reusable per-facet controls (`FacetSection`, `PillGroup`, `SearchSelect`).

## Goals / Non-Goals

**Goals:**
- Make the companies catalog browsable by collection, region, country, industry,
  company type, and company size.
- Derive those facets onto the company from its open jobs and filter with plain
  SQL, staying consistent with the existing `job_count` denormalization pattern.
- Reuse the existing frontend filter machinery rather than inventing a parallel one.

**Non-Goals:**
- Live facet counts next to company options (jobs-style distribution). Options come
  from static vocabularies; counts are a later, separable addition.
- A Meilisearch index for companies. The list stays plain SQL.
- Real (external) company headcount. `company_size` is the LLM's per-job estimate,
  used as-is.
- Synchronous, per-write facet maintenance. Eventual consistency via the recompute
  is sufficient (same guarantee `job_count` already has).

## Decisions

### D1: Denormalize facet arrays onto `companies`, filter by array overlap

Add five `TEXT[]` columns to `companies` — `regions`, `countries`, `domains`,
`company_types`, `company_sizes` — each the distinct union of the value across the
company's open jobs. Filter the list with the Postgres array-overlap operator
`&&`: OR within a facet, AND across facets. `collections` (already an array) joins
the same filtering path.

*Why over alternatives:* query-time aggregation (JOIN to `jobs` on every list
request) is heavier and yields no clean per-company facet value set; a Meili
companies index is a whole new index + reindex worker — overkill for a
closed-vocabulary browse filter. Denormalize-and-recompute mirrors `job_count` and
the project's established "derive a fact next to the row, refresh periodically"
pattern.

### D2: Extend the existing recompute pass, don't add a worker

Fold the facet aggregation into `RecountCompanyJobCounts` (renamed to reflect its
wider job, e.g. `RefreshCompanyFacets`) so one set-based `UPDATE … FROM` computes
`job_count` **and** all five arrays in a single pass, guarded by `IS DISTINCT FROM`
so unchanged rows aren't rewritten. `cmd/recount-companies` keeps its shape (load
pool → one query call → log affected rows).

The aggregation subquery groups open jobs by `company_slug`:
- `regions` / `countries`: `array_agg(DISTINCT r ORDER BY r)` over
  `unnest(jobs.regions|countries)`.
- `domains`: `array_agg(DISTINCT d ORDER BY d)` over
  `jsonb_array_elements_text(enrichment->'domains')`.
- `company_types` / `company_sizes`: `array_agg(DISTINCT enrichment->>'company_type'|'company_size')`
  filtered to non-null, non-empty.

Arrays are aggregated with a stable `ORDER BY` so the `IS DISTINCT FROM` guard
compares deterministically (element order is significant to array equality).

*Why:* a second worker/cron would duplicate the load-pool + schedule scaffolding
and split a single logical "refresh the company's derived state" into two passes
that can disagree between runs. One pass, one guard, one cron entry.

### D3: Extract a `FacetStore` interface; add `COMPANY_FACETS` + a thin store & panel

Introduce a `COMPANY_FACETS` registry (a subset of the jobs `FacetDef` shape:
`collections`, `regions`, `countries`, `domains`, `company_type`, `company_size`,
reusing the existing `COLLECTION` / `REGION` / `DOMAIN` / company-type option lists,
plus a static `COMPANY_SIZE` list and a static ISO `COUNTRY` list) and a thin
`CompanyFiltersPanel` that iterates it and renders each via the existing
`FacetSection`. `CompaniesView.svelte` gains the sidebar next to the list.

The job `FilterStore` is **not** reused directly — it is coupled to the global jobs
`FACETS` registry and the job-only shape (`visa`/`salary`/`sort`), and it serializes
the `_exclude`/`_mode` conventions the companies endpoint doesn't understand.
Instead, extract the narrow `FacetStore` interface `FacetSection` actually depends on
(`facet`/`toggle`/`add`/`remove`/`clearFacet`/`setExclude`/`setMatchAll`), retype
`FacetSection` to it, and add a dedicated `CompanyFilterStore` — a thin wrapper over
the same shared `UrlSyncedState` primitive with plain repeated-param
(de)serialization (no exclude/mode). Both stores satisfy `FacetStore`, so the same
section/control components render either.

*Why:* `FacetSection` already renders pills/selects, handles clear, and is
closed-vocabulary-aware; the `UrlSyncedState` primitive already solves URL sync and
back/forward restoration (the exact bug class fixed in PR#309). Reusing them via a
shared interface keeps one implementation of those hazards. We deliberately do **not**
reuse the jobs `FiltersPanel` wholesale because it hard-codes job-only controls
(salary slider, freshness, remote/visa checkboxes). `country` uses the
searchable-select control over the static ISO country list (no live counts).

### D4: SSR forwards facet params

`/companies/+page.server.ts` reads the facet params from the request URL and passes
them to `listCompanies` so the first render is already filtered (matching how `?q`
is handled today), preserving shareable/deep-linkable filtered URLs.

## Risks / Trade-offs

- **Sparse enrichment facets** (`domains`/`company_types`/`company_sizes` empty
  until jobs are enriched) → acceptable: geography facets are dense from ingest, and
  enrichment backfills over time. Document it; don't block on full enrichment.
- **`company_size` is an LLM estimate**, and a company's jobs may disagree (so a
  company can appear under multiple size buckets) → acceptable for a browse filter;
  overlap semantics already tolerate multiple values.
- **No live counts** → a user can pick a facet combination that yields zero
  companies with no forewarning. Mitigation: the existing empty-state handles it;
  live counts are a noted future addition.
- **Array-equality churn in the guard** if aggregation order isn't stable → mitigated
  by `ORDER BY` inside every `array_agg`, so equal sets compare equal.
- **Migration on a live prod volume**: initdb-only migrations don't re-apply, so the
  new columns must be added manually via psql on prod (established ops), then one
  `cmd/recount-companies` run backfills. Rollback: the columns are additive and
  unread by old code paths; dropping them (or leaving them) is safe.

## Migration Plan

1. Ship additive migration adding the five `TEXT[]` columns (`DEFAULT '{}'`).
2. On prod, apply the migration manually via psql (initdb migrations don't
   re-apply to an existing volume).
3. Deploy backend (recompute now refreshes facets; list endpoint accepts facet
   params) and frontend (sidebar).
4. Run `cmd/recount-companies` once to backfill the arrays; thereafter the existing
   cron keeps them fresh. No reindex (companies list is not Meili-backed).

Rollback: revert app to the prior build; the extra columns are inert. No data
migration to undo.

## Open Questions

None — scope (facets, storage, counts-later, country in v1, company_size included)
was settled during brainstorming.
