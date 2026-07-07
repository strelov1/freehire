## Why

People don't search by an abstract taxonomy value like `backend` — they think in
natural role names ("Senior Backend Engineer", "Founding Engineer", "Cloud
Solutions Engineer"). Today the only role controls are two separate primitive
facets (seniority pills + grouped category chips) that can't express a named role
and force the user to translate their mental model into the taxonomy. We add a
single natural "role" picker backed by a new derived facet, so users select
familiar roles directly and get precise results.

## What Changes

- Add `internal/roletag`, a deterministic dictionary (mirroring `classify`/
  `skilltag`) that derives a job's `roles []string` from its `(seniority,
  category, title)`:
  - the **bare category role** `{category}` (e.g. `backend`, `data_science`)
    whenever the category resolves — the dominant case (only ~18% of live prod
    jobs carry a seniority, so the composite's both-axes rule left most jobs
    role-less; bare category roughly doubles coverage, up to `classify`'s ~30%
    category-resolution ceiling);
  - the composite `{seniority}_{category}` (e.g. `senior_backend`) in addition
    when the seniority also resolves;
  - named-role alias matches from the title for roles that don't fit the grid — a
    `software_engineer` catch-all plus a set curated from the same prod-title
    mining across departments (`founding_engineer`, `fractional_cto/cfo/cmo/…`,
    `cloud_solutions_engineer`, `sdr`, `bdr`, `product_marketing_manager`,
    `developer_advocate`, `technical_recruiter`, …);
  - nothing for what it can't resolve (never guesses).
- Compute `roles` **at index time** in `search.FromJob` — no `jobs.roles`
  column, no migration, no backfill; a reindex populates it (same pattern as the
  existing derived `posted_ts`).
- Wire `roles` as a Meilisearch **filterable + faceting** attribute; add a public
  search/facets query param `role` → index attribute `roles`, ORed within the
  facet like `skills`, with `role_exclude` / `role_mode=and` support; expose
  `roles` in the `/api/v1/jobs/facets` distribution so the picker gets live
  busiest-first counts.
- `cmd/gen-contracts` emits the role catalog (slug → label) from `roletag` into
  the web contracts, the source of truth for picker labels.
- Frontend adds a **dynamic** `role` facet control (typeahead, live counts,
  `hasAndOr`, excludable) reusing the existing `skills` `FacetSection` path. The
  existing seniority and category controls **stay** for now — the picker is
  additive, not a replacement.

Not in this change (deliberate): a `jobs.roles` column + `backfill-derive`
support; removal of the old seniority/category controls; any query-time
title→facet decomposition. Each is a follow-up if the picker proves out.

## Capabilities

### New Capabilities
- `role-facet`: the `roletag` deterministic role dictionary (composite +
  named-role derivation, never-guess), its curated catalog (slug/label/group)
  exported to the web contracts, and the rule that `roles` is derived at index
  time and served as an additive search facet.

### Modified Capabilities
- `job-search`: the jobs index gains a filterable + facetable `roles` attribute
  derived at index time, and the search / facets endpoints gain a `role` filter
  param (with `_exclude` / `_mode`) and `roles` facet distribution.
- `filter-modal`: the filter UI gains a dynamic, count-driven "Role" picker
  control alongside the existing seniority and specialization controls.

## Impact

- **New code**: `internal/roletag` (dictionary + catalog + tests).
- **Backend**: `internal/search` (`FromJob` derivation, filterable/faceting
  settings, `StringFacets` mapping), `internal/handler/facets.go` distribution,
  `cmd/gen-contracts` catalog emission. No schema/migration change.
- **Frontend**: `web/src/lib/contracts.ts` (generated role catalog),
  `facets.ts` / `labels.ts` / filter section wiring, one new dynamic facet
  control reusing the `skills` component path.
- **Ops**: a reindex after deploy populates `roles` on existing documents; the
  old filter keeps working throughout (backward compatible, no URL breakage).
