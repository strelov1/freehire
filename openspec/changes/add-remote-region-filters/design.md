## Context

Geography is a deterministic dictionary (`internal/location`) feeding Meilisearch
facets (`regions`/`countries`/`work_mode`); it never guesses and emits nothing it
cannot resolve. The `global` region already exists and several open-anywhere
markers (`Anywhere`, `Worldwide`, `Global`, …) already map to it, but
`International` does not. Separately, a bare `Remote` deliberately yields no
region — correct (such a posting is often silently region-restricted), but it
leaves a large set of remote jobs unreachable from the Region filter. The
employment-type matcher already resolves `Contractor`/`freelance`/etc. to
`employment_type=contract` with an existing filter and UI pill, so `Contractor`
needs only a data backfill.

All facets follow one pattern: derive at ingest via `jobderive`, store a `jobs`
column beside (not inside) the `enrichment` JSONB, serve dict-only via `jobview`,
index as a Meilisearch facet. A dictionary/derivation change reaches existing rows
only via `cmd/backfill-derive` + `make reindex`.

## Goals / Non-Goals

**Goals:**
- Recognize `International` (and close worldwide synonyms) as `global`.
- Add a `remote_unspecified` facet (remote + no resolved geography), filterable in
  the API and selectable in the SPA, with a facet count like other Region pills.
- Reach existing rows through the existing re-derive + reindex path.

**Non-Goals:**
- Mapping a bare `Remote` to `global` (explicitly rejected — keeps `global` clean).
- `EMEA` handling (deferred).
- Any `Contractor` code change (already implemented; backfill-only).
- Any LLM/enrichment change.

## Decisions

**1. `International` → `global` as a dictionary entry.** Add `international` (and
close synonyms: e.g. `international remote`, `globally`) to `location.nameToRegion`
→ `global`. *Alternative considered:* a new region value — rejected; `International`
means "the whole world", which is exactly what `global` already encodes. The
existing `RegionValues`-membership invariant test covers it for free.

**2. `remote_unspecified` as a stored boolean column, not a computed query
predicate.** Add `Derived.RemoteUnspecified` computed in `jobderive.Derive` as
`work_mode == "remote" && len(countries) == 0 && len(regions) == 0`, persisted to
a new `jobs.remote_unspecified BOOLEAN NOT NULL DEFAULT false` column, served by
`jobview`, and registered as a Meilisearch filterable attribute. *Alternative
considered:* a compute-only compound filter (`work_mode="remote" AND regions IS
EMPTY AND countries IS EMPTY`) with no column — rejected: it cannot produce a
facet-distribution count for the pill, and it scatters a magic predicate in the
filter builder. A stored, named facet matches the codebase's facet pattern and
gives the SPA a count badge consistent with the other Region pills.

**3. Filter param mirrors `visa_sponsorship`.** Boolean facets in this codebase
are not in the `StringFacets` map; they are handled explicitly in
`FilterFromValues` via `EqBool` (see `visa_sponsorship`). `remote_unspecified`
follows the same shape: `if v.Get("remote_unspecified") == "true" { … EqBool(...) }`.
This keeps the pure filter builder shared by the HTTP handler and the saved-search
matcher.

**4. SPA pill in the Region group, distinct from Global.** Render a control in the
Region facet group bound to the `remote_unspecified` param, labelled to read as
"remote, region not specified". It is a boolean, not a member of the `regions`
array, so it stays a separate control rather than polluting `REGION` /
`RegionValues`.

## Risks / Trade-offs

- [New filterable attribute 500s `/jobs` until reindex] → Deploy ordering: add the
  attribute and run a full fresh-index + swap reindex BEFORE the API/UI exposes the
  param. This recurred before; treat reindex as a precondition, not a follow-up.
- [Stored boolean is redundant with `work_mode`/`regions`/`countries`] → Accepted:
  it is always derived from those three by `jobderive` (never hand-set), so it
  cannot drift; the redundancy buys a clean named facet + count.
- [`remote_unspecified` over-broad] → It is intentionally narrow (requires
  `work_mode=remote`); a region-tagged remote job (incl. `global`) is excluded, so
  it captures only the truly unspecified bucket.
- [Backfill scope] → `cmd/backfill-derive` already re-derives all dictionary facets
  in one pass; it must also write the new column (and picks up the `International`
  mapping and the pre-existing `Contractor` matcher in the same run).

## Migration Plan

1. Add the column migration; recreate the volume in dev (`down -v && make up`) per
   the initdb-only migration convention, or apply manually in prod.
2. `make sqlc` after editing queries; commit generated code.
3. Ship the derivation + storage + reindex config; run `cmd/backfill-derive` then a
   full `make reindex` (fresh index + swap) so the new attribute exists before the
   API/UI references it.
4. Then enable the API param and SPA pill.

Rollback: drop the SPA pill / API param first; the column and attribute are inert
if unfiltered. The column can remain (defaults false) without effect.

## Open Questions

None — scope and approach confirmed during brainstorming.
