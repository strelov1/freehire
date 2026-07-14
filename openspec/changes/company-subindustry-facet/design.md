## Context

`companies.industries` (`TEXT[]`) flattens YC's `industry` (top-level, ~8), `industries[]`,
`subindustry` leaf, and free `tags` into one array, and is used only for display (chips on the
company page, first value on the card/OG image). The team's published position is that this
flattened array is "too noisy to trust" for classification — the noise being the unbounded
tags. The company "Industry" filter is actually the job-derived `domains` facet (14 coarse
buckets), which does not use `industries` at all. YC's `subindustry` (a single path per company,
~100 bounded leaves) is the clean, structured slice, and is the natural rich industry axis.

## Goals / Non-Goals

**Goals:**
- Let users filter companies by their clean YC subindustry via a searchable facet.
- Keep the noisy tags out of the filter while leaving the display bag untouched.

**Non-Goals:**
- Classifying non-YC companies into the taxonomy (an LLM project deliberately avoided);
  coverage is YC-only.
- Touching `companies.industries` (display) or the job-derived `domains` facet's data.
- Conditional (filter-aware) facet counts like the Meilisearch job facets.

## Decisions

**Store the subindustry as a clean scalar column (`companies.subindustry TEXT`, nullable),
separate from `industries`.** The noise lives in tags; separating the structured leaf into its
own column is the structural answer to "industries are noisy" — the filter reads the clean
column, the display keeps the full bag. Scalar (not array) because YC assigns one subindustry
per company. *Alternative rejected:* faceting on the existing `industries` column with a
curated option list — cheaper but re-admits tag contamination at filter time and offers no
clean provenance.

**Filter by membership, mirroring `maturity`.** The `subindustries` param filters
`subindustry = ANY($)`, NULL matches nothing — an exact copy of the existing scalar `maturity`
facet, so no new filtering pattern is introduced.

**Serve the option vocabulary from a new dynamic endpoint
`GET /api/v1/companies/subindustries` (value + count).** Companies have no facet-values
endpoint today (job facets use Meilisearch; company facets are static frontend lists), so this
is net-new. Chosen over a static generated TS list because a ~100-item list is better with live
counts and self-updates as `import-yc` runs, and subindustry leaves are already human-readable
(no label map needed). Counts are unconditional — a deliberate simplification; conditional
faceting over SQL is out of scope.

**Relabel the existing `domains` facet "Industry" → "Domain".** Two distinct axes now exist
(job-derived domain vs YC industry); relabelling avoids two "Industry" filters. Data and param
of `domains` are unchanged — label only.

**Populate via `cmd/import-yc`; backfill by re-running it.** `ycdir.Map` gains
`Record.Subindustry = subindustryLeaf(e.Subindustry)`; `UpsertYCCompany` writes it. The upsert
is idempotent, so re-running `cmd/import-yc` backfills existing rows — no separate backfill
tool.

## Risks / Trade-offs

- **YC-only coverage** → accepted and stated in the facet's framing; NULL is never guessed, so
  non-YC companies simply aren't matched (honest, not wrong).
- **Unconditional counts can overstate availability under other active filters** → acceptable
  for a discovery filter; the list result itself is always correctly filtered.
- **New company facet-values endpoint is a new pattern** → small (one query + handler + route);
  a reasonable seam for future company facets.

## Migration Plan

- Add `companies.subindustry TEXT` migration; apply manually to prod before deploy (migrations
  convention — no versioned runner).
- Regenerate `internal/db` via `make sqlc`.
- Post-deploy: re-run `cmd/import-yc` to populate `subindustry`. No reindex (SQL filter).
- Rollback: the column and endpoint are additive; reverting the frontend facet hides it, and the
  column can be left in place harmlessly.

## Open Questions

- Whether to surface `subindustry` in the company API payload for display later (out of scope
  now; the facet does not require it).
