## Why

A remote job's *geographic reach* matters — open worldwide, restricted to a
region (EU), or to specific countries (US) — but the model can't express it. Reach
is captured only by `enrichment.countries` (ISO codes), so "open anywhere"
(global) and "reach not stated / not yet enriched" (unknown) both collapse to an
empty `countries` array and become indistinguishable, and regions like EU have no
representation at all (they are not ISO countries). Meanwhile the raw
`jobs.remote` boolean duplicates `work_mode=remote` as a parallel public concept,
forcing flag-vs-enrichment reconciliation.

## What Changes

- Capture a remote role's **reach in a single `regions` field** on the enrichment
  contract — an enum array over one controlled vocabulary that mixes levels:
  `global` (open anywhere) + macro-regions (`eu`, `emea`, `eea`, `uk`,
  `americas`, `north_america`, `latam`, `apac`, `mena`, `africa`) + select
  countries treated as reach areas (`us`, `ru`, extensible). There is **no
  separate scope discriminator**: empty `regions` = unknown, and `global` is an
  explicit value (never inferred), so **global ≠ unknown**. `regions` is
  meaningful only when `work_mode = remote`. "remote" itself stays purely the
  `work_mode` format — nothing else is hung on it.
- Validate each `regions` element against the vocabulary (reject out-of-vocab →
  dead-letter, as today).
- Extend the LLM extraction prompt to populate `regions` (`global` only on an
  explicit "worldwide/anywhere"), only when `work_mode = remote`.
- Register `enrichment.regions` as a filterable search attribute and wire a
  `regions` search-filter param — filtered directly via its dot path, like every
  other enrichment facet. **No derived field.**
- SPA: add a curated **"Region"** pills facet (Global / Russia / Europe / USA,
  excludable) under "Work format" bound to `regions`, and display a remote job's
  reach from `regions` instead of showing nothing for global.
- **BREAKING (internal, pre-launch):** demote `jobs.remote` from a public field
  to an internal enrichment hint. Remove it from the public job wire
  (`jobview.Job.remote`), from the search filter (`?remote=true`) and filterable
  attributes, and from the SPA fallback. The **column stays**, populated by every
  adapter and fed to the LLM as the source's explicit remote signal. Public
  "remote" is henceforth solely `work_mode` (+ `regions` for reach).
- No `enrich.Version` bump and no DB migration: at MVP there is no persistent
  data, so fresh ingest + enrich fills the new fields; recreate the dev volume if
  needed.

## Capabilities

### New Capabilities
<!-- None. The change extends existing enrichment, search, and frontend
     capabilities rather than introducing a new one. -->

### Modified Capabilities

- `job-enrichment`: the typed enrichment contract gains a single `regions`
  enum-array reach field (vocabulary incl. `global`, macro-regions, and select
  countries; validation); the public jobs read API drops the now-redundant raw
  `remote` field (subsumed by `work_mode`).
- `job-search`: the index filterable attributes drop `remote` and add
  `enrichment.regions`; the search endpoint replaces the `remote` filter with a
  `regions` facet filtered via its dot path.
- `web-frontend`: list and detail views express a job's remote reach from
  `enrichment.regions` rather than the raw flag; the filter UI gains a curated
  "Region" facet.

## Impact

- **Code (enrichment):** `internal/enrich/enrichment.go` (the `regions` field +
  vocabulary + `Validate`), `internal/enrich/langchain.go` (prompt). No new
  dependency.
- **Code (search):** `internal/search/client.go` (filterable attrs: −`remote`,
  +`enrichment.regions`), `internal/handler/search.go` (param mapping: −`remote`,
  +`regions`). No derived document field.
- **Code (wire):** `internal/jobview/jobview.go` (`regions` rides the embedded
  Enrichment; remove `remote`); sqlc unaffected — column stays.
- **Code (frontend):** `web/src/lib/types.ts`, `web/src/lib/facets.ts`,
  `web/src/lib/enrichment.ts`, `JobView.svelte`/`JobRow.svelte`.
- **Kept untouched:** `jobs.remote` column, `sources/*` + `pipeline` `Remote`
  field, `enrich.Provider` input, `cmd/ingest`/`cmd/enrich` — the remote hint
  path. No migration, no `enrich.Version` change.
- **Tests:** `Enrichment.Validate` regions cases; search regions filter; jobview
  omits `remote`; frontend display.
