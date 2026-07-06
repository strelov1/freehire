## Context

Role filtering today is two primitive facets: seniority pills and grouped
category chips (`web/src/lib/components/filters/CategoryPane.svelte`,
`ChipFacet.svelte`), backed by the `enrichment.seniority` / `enrichment.category`
index attributes. Free-text `q` goes only to Meilisearch full-text; it is never
decomposed into structured facets. Users think in natural role names ("Senior
Backend Engineer", "Founding Engineer") that the taxonomy can't express.

The codebase already has the machinery to solve this idiomatically: deterministic
dictionaries (`internal/classify`, `internal/skilltag`) derive facet columns; the
search index is built by `search.FromJob`; multi-valued facets (`skills`) filter
as an ORed IN-list and are exposed with live counts via `GET /api/v1/jobs/facets`
and rendered by a dynamic `FacetSection`. The `posted_ts` field establishes the
precedent for an index-only derived field with no column or backfill.

## Goals / Non-Goals

**Goals:**
- A natural, multi-select "Role" picker backed by a single `roles` facet.
- Precise multi-role selection with no cross-product garbage.
- Live busiest-first counts and typeahead, reusing the existing `skills` path.
- Minimal footprint: no schema change, additive to the existing filters.

**Non-Goals:**
- A `jobs.roles` column, migration, or `backfill-derive` support (follow-up).
- Removing the seniority/category controls (follow-up once the picker proves out).
- Query-time title→facet decomposition of the `q` box.
- Serving `roles` in the public job read shape.

## Decisions

**Precompute, don't parse at query time.** A role is a precomputed tag, so the
filter is a plain ORed IN-list over one `roles` facet — exactly like `skills`.
This eliminates the OR-of-AND-groups problem that a query-time `role`→facet
expansion would need: `role=senior_backend&role=lead_frontend` matches jobs
tagged with either, never `senior_frontend`. No filter-builder machinery is
added; `roles` slots into `StringFacets` as one more entry.

**Compute at index time, no column.** `roles` is a pure function of the job's
already-derived `seniority` + `category` columns and its `title`, so
`search.FromJob` computes it directly. Following the `posted_ts` precedent, this
needs no `jobs.roles` column, no migration, and no backfill — a reindex populates
existing documents. If we later need roles in the served wire shape or want to
retire the old facets, promoting to a column is a clean follow-up.

**Derivation rules (`internal/roletag`).** `Derive(seniority, category, title)
[]string`:
- composite `{seniority}_{category}` iff both inputs are non-empty;
- named-role alias matches from the title via whole-word matching
  (`wordmatch.Contains`, unicode boundary — same as `classify`), for roles that
  don't fit the grid;
- dedupe; never guess. The package also exports the catalog (slug→label,group).

**Catalog is the source of truth, emitted to contracts.** `roletag` owns the
canonical list (composite labels like "Senior Backend Engineer" are generated
from the seniority/category labels; named roles are curated). `cmd/gen-contracts`
emits it into `web/src/lib/contracts.ts`, matching how `CATEGORY_VALUES` is
generated. The frontend adds one `FACETS` entry `role` with `dynamic:true`,
`hasAndOr:true`, excludable, reusing `FacetSection` → `counts.facets.role`.

**Additive rollout.** The `role` control is added to the ROLE rail section
alongside the existing seniority and specialization controls. Old URL filters
keep working; a post-deploy reindex lights up the new facet.

## Risks / Trade-offs

- **Facet cardinality.** Composite (≤8×26) plus named roles is a large value
  space, but many composites are empty and live counts sink them; `skills` is
  already larger and `MaxValuesPerFacet` is raised. Acceptable.
- **Redundant tagging vs the old facets.** While both live in parallel, a Senior
  Backend job is reachable via `role=senior_backend` and via
  `seniority=senior`+`category=backend`. That's the intended transition state,
  not a bug.
- **Broad single-axis roles.** "Any senior across categories" is not a `roles`
  value in this change; it stays served by the retained seniority facet. If the
  old facets are later removed, the catalog must add broad roles or the picker
  must special-case them — deferred.
- **Reindex dependency.** The facet is empty until the post-deploy reindex
  completes; the old controls cover the gap. Standard "dictionary change →
  reindex" caveat.
