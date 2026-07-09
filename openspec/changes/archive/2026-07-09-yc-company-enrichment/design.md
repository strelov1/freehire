## Context

`internal/collections.ParseYC` fetches yc-oss `all.json` and reads only `name`
(→ `collections=yc`). The payload has ~25 fields per company. `UpsertCompanyInfo`
already writes the company-info columns (`tagline`, `industries`, `year_founded`,
`employee_count`, `hq_country`, `organization_type`, `company_info` JSONB) and
inserts a reference row (`is_reference=true`) for an unmatched slug. So enriching
from yc-oss is a mapping + two curated facet columns, not new infrastructure.

## Goals / Non-Goals

**Goals:**
- Hold the full YC directory: update matched companies, insert the rest as
  reference rows.
- Map yc-oss's descriptions/industry/size/founded/HQ into existing company-info.
- Add `yc_batch` + `yc_status` as filterable company facets.

**Non-Goals:**
- LLM enrichment — yc-oss is authoritative, deterministic.
- Touching job-derived facets or the recompute (the new columns are curated).
- A batch-code normalizer beyond what the source gives (store `Winter 2012` /
  `Summer 2020` verbatim as the facet value; the UI lists them).

## Decisions

**Dedicated `UpsertYCCompany`, not an extended `UpsertCompanyInfo`.** YC carries
two extra curated columns (`yc_batch`, `yc_status`) and its own provenance; a
separate `:exec` query (insert reference / update company-info + yc facets by slug)
keeps the generic company-info upsert uncoupled. It writes the same company-info
columns plus the two facets and `is_reference=true` on insert.

**`yc_batch`/`yc_status` are `text[]`, single-element.** A company has one batch
and one status, but modelling them as `text[]` reuses the exact `&&` overlap facet
machinery (`ListCompanies`, the FilterModal) with zero new filter logic. They are
**curated** (importer-owned) and the recompute never references them — the same
exemption `remote_regions` used to have, verified by a guard test.

**yc-oss mapping in `internal/ycdir`.** A small package with the full entry struct
and a pure `Map(entry) → fields` (tagline, industries = `industry`+`tags` deduped,
year from `launched_at`, `employee_count` from `team_size`, `hq_country` via
`location.Parse(all_locations)`, and the `company_info` JSONB extras). Keeps
`internal/collections` focused on names and makes the mapping unit-testable.

**Reuse `internal/location` for HQ.** `all_locations` ("London, England, United
Kingdom") → `location.Parse(...).Countries[0]` — no new geo dictionary.

## Risks / Trade-offs

- **~6k reference cards** in the companies catalogue, many job-less. → Accepted by
  decision; `is_reference` rows are preserved by `DeleteOrphanCompanies` and a
  later YC job for the same slug adopts the row.
- **Two curated sources** (`backfill-company-info` + `import-yc`) can both write a
  company's company-info. → For a YC company, whichever ran last wins; acceptable
  (they largely partition, and YC data is authoritative for YC companies).
- **Batch string drift** (source could change wording). → We store verbatim; a
  wording change just adds a new facet value, harmless.

## Migration Plan

1. Ship migration (`yc_batch`, `yc_status`) + code; regenerate sqlc.
2. Deploy (column exists before the binary reads it, per the repo convention).
3. Run `cmd/import-yc` once on host to load the directory + facets.
4. Rollback: revert binary; the additive columns and reference rows are harmless;
   drop columns only if ever needed.

## Open Questions

- None blocking. A YC-batch shorthand normalizer (`Winter 2012` → `W12`) and a YC
  "top company"/"is hiring" surfacing are deferred.
