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
- the **bare category role** `{category}` whenever the category resolves (any
  `enrich.CategoryValues` except `other`) — this is the dominant case: on the
  live prod catalogue only ~18% of open jobs carry a seniority, so requiring one
  (as the composite does) leaves most jobs role-less. Bare category roughly
  doubles role coverage, up to the ceiling set by `classify`;
- the composite `{seniority}_{category}` **in addition** when the seniority also
  resolves — the graded role layered on the bare one;
- at most one named-role alias match from the title via whole-word matching
  (`wordmatch.Contains`, unicode boundary — same as `classify`), longest alias
  first so the most specific wins, for roles that don't fit the grid (incl. the
  `software_engineer` catch-all, the largest category-less bucket in the sample);
- distinct slug namespaces so no dedup step is needed; never guess. The package
  also exports the catalog (slug → label). No display group — the picker is a
  flat busiest-first typeahead like `skills`.

The named-role set was curated from that same prod-title mining (throwaway tool),
not guessed: bare category covers the generic roles across every department, and
named roles fill the distinctive titles the grid flattens.

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
- **Role facet is the primary role axis, bounded by `classify`.** With bare
  category roles the `roles` facet subsumes the category facet at the
  any-seniority level (a bare "Data Scientist" → `data_science`) and stays
  additive. But its coverage ceiling is `classify`'s category resolution: on the
  live catalogue only ~30% of open jobs (899k / 2.98M) get a category, so the
  role facet tags roughly a third of jobs, not most of them. Raising that further
  is a `classify` dictionary-expansion effort (the title mining feeds it), not a
  `roletag` change. (An earlier "85%" figure was a stratified-sampling artifact —
  the sample was drawn via `category=` filters, so it was categorized by
  construction; corrected here.)
- **Reindex dependency.** The facet is empty until the post-deploy reindex
  completes; the old controls cover the gap. Standard "dictionary change →
  reindex" caveat.
