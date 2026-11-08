## Context

`companies` rows carry denormalized facet arrays (`regions`, `countries`,
`domains`, `company_types`, `company_sizes`) that `RefreshCompanyFacets`
(`cmd/recount-companies`) rebuilds from the company's open jobs on a schedule.
These describe *where our crawled postings sit*. Separately, a curated public
directory (the Atul Kumar remote-companies PDF) declares, per company, the
regions where the company *hires remotely* — a company-level fact that no job we
hold can derive.

The project already has a precedent for loading an external company dataset:
`cmd/backfill-company-info` + `UpsertCompanyInfo` matches records by
`normalize.Slug(name)` and writes only company-info columns, deliberately never
touching the job-derived facets. This change follows that shape but adds a
dedicated, filterable column instead of a JSONB blob, because the value is a
first-class facet users filter on.

## Goals / Non-Goals

**Goals:**

- Persist per-company "hires remotely in region X" as a filterable macro-region
  array that survives the facet recompute.
- Reproducible, source-documented backfill (checked-in dataset + pure mapping).
- Expose it as a company-list facet param and a UI filter.

**Non-Goals:**

- Creating reference rows for PDF companies absent from our DB (annotate existing
  only). If we later want a remote-employer directory, that is a separate change.
- Website/domain matching. Slug-only; the domain fallback is a noted seam.
- Merging the PDF regions into the job-derived `companies.regions` facet.
- Re-running on a schedule. This is a one-time (repeatable) load, not a cron.

## Decisions

**Dedicated column over `company_info` JSONB.** The value is a facet users filter
by (array overlap), matching the existing `regions`/`countries` filter mechanics.
A JSONB path can't be filtered with the same `&&` overlap predicate without
extraction/indexing. Cost: one migration. The raw source string still lands in
`company_info.remote_regions_raw` (no filtering need — audit only), so the JSONB
carries the provenance and the column carries the queryable normalized set.

**Recompute must skip `remote_regions`.** `RefreshCompanyFacets` rewrites the
five job-derived arrays via `IS DISTINCT FROM` guards; it simply never references
`remote_regions`, so a curated value persists. This is an invariant to encode in
the `companies` spec and guard with a test (recompute leaves `remote_regions`
untouched).

**Pure mapping dictionary in its own package.** `internal/remoteregion.Map` is a
pure function (`raw → []string` from `enrich.RegionValues`), mirroring
`internal/location`'s "curated dictionary, never a geocoder" design — but
**best-effort** (maps timezone/narrow-geo to the nearest macro region) rather than
strict, per the product decision. Isolating it makes the ambiguous business logic
independently unit-testable and keeps the worker thin. The dataset's `Region`
column is a **closed set of 108 distinct strings** (enumerated from the checked-in
CSV), so the dictionary is finite and every source string is a test case rather
than an open-ended NLP problem. Composite strings are split on `,` and `/` and
mapped component-wise, then de-duplicated. Resolved judgment calls (durable):

- **`Ukraine` → `cis`** (not `eu`); so `Ukraine, Poland` → `{cis, eu}`.
- **Wide timezone spans** covering the Americas through Europe (`UTC-8 to UTC+2`,
  `UTC-10 to UTC+2`, `UTC-5 to UTC+3`, `UTC-9 to UTC+1`, `UTC -3 to -5`) →
  `{north_america, eu}` (the two nearest macro regions), not `global` and not
  empty. Narrow single-offset zones map to their one region (`UTC+2` → `eu`).
- **`EMEA`** expands to `{eu, mena, africa}`; `East American / European / African
  timezones` → `{north_america, eu, africa}`.
- **`UK`** resolves to the distinct `uk` region, never `eu`.

**UPDATE-only upsert.** `SetCompanyRemoteRegions` updates by slug and reports rows
affected; an unmatched slug is a no-op (counted, not inserted). This is the
"annotate existing only" decision. It also means the query never sets
`is_reference` and never touches name/job_count/collections/job-derived facets.

**Dataset format: CSV under `sources/`.** `sources/remote-companies.csv` with
header columns `Name`, `Website`, `Region` — exported externally from the source
directory and checked in as-is, not transcribed in this change. CSV is the format
the source is provided in; the worker parses it with `encoding/csv`. `sources/` is
where external input files live. The `Region` cell is kept verbatim
(`Worldwide`, `Europe, Americas`, `USA East Coast`, …); normalization to
macro-region codes is `remoteregion.Map`'s job, not the dataset's.

## Risks / Trade-offs

- **Mapping is lossy / opinionated** (e.g. `Americas → [north_america, latam]`,
  `Western Asia → mena`). → Keep the raw string in `company_info.remote_regions_raw`
  so any mapping call is auditable and re-runnable after a dictionary fix; unit
  tests pin the intended mappings.
- **Slug mismatch misses companies** whose PDF name normalizes differently than
  our stored slug. → Accept for v1; the worker's `unmatched` count surfaces the
  gap, and a domain-fallback seam is documented for later.
- **Recompute regression** could accidentally clobber the column if a future edit
  adds `remote_regions` to `RefreshCompanyFacets`. → A test asserts the recompute
  leaves a curated value intact.
- **Stale data**: the PDF is a point-in-time snapshot. → Backfill is idempotent
  and re-runnable from an updated dataset; no staleness machinery in scope.

## Migration Plan

1. Ship migration adding `companies.remote_regions text[] NOT NULL DEFAULT '{}'`
   (apply before deploying the binary that reads it — per the repo's
   unapplied-migration convention).
2. Deploy the new query/handler/UI (empty column reads as no-op facet — safe).
3. Run `cmd/backfill-remote-regions sources/remote-companies.jsonl` once.
4. Rollback: the column defaults empty and the facet is additive; reverting the
   binary leaves the column harmless. Dropping the column is a follow-up migration
   if ever needed.

## Open Questions

- None blocking. Domain-fallback matching and a full remote-employer directory
  (reference rows) are deferred by explicit decision, not open questions.
