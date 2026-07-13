## Context

freehire derives deterministic facets (geography, skills, seniority, category) from a job's title/description via curated whole-word dictionaries (`internal/classify`, `internal/location`, `internal/skilltag`) at ingest, persisted on `jobs.*` columns, served through `jobview`, and indexed as Meilisearch facets. `category` already distinguishes 4 confidently non-tech buckets (marketing/sales/support/management) — the `enrich.NonTechCategories` blacklist that gates AI enrichment. But generic ATS boards flood the catalogue with non-tech roles the title dictionary cannot place: on prod, 69% of ~3.07M open jobs have an empty `category`, a mix where ≥0.5M titles match obvious non-tech nouns. This change adds a confident non-tech title detector and a tri-state `is_tech` signal, exposed as a filterable facet.

## Goals / Non-Goals

**Goals:**
- Deterministically flag confidently non-tech titles beyond the 4 blacklist categories, never guessing.
- Derive a tri-state `is_tech` (`true`/`false`/unknown) that stays honest about coverage.
- Expose `is_tech` as a search facet with a Tech / Non-tech filter in the web UI.
- Persist `is_tech` on the job row so the split is measurable with plain SQL on prod.

**Non-Goals:**
- Excluding non-tech from the catalogue or search index (a later decision, made on the measured split).
- Gating `cmd/enrich` / `cmd/embed` on `is_tech` (the existing category blacklist still gates them).
- Description-based non-tech detection (title-only first; add prose later only if the numbers demand it).
- Any change to the `category` vocabulary or its facet — `is_tech` is a separate, additive signal.

## Decisions

**1. A separate non-tech detector, not new `category` values.**
Add `classify.IsNonTech(title) bool` backed by a new `nonTechTable` of confident non-tech role nouns (whole-word via `wordmatch.UnicodeBoundary`, EN first). Rationale: keeping it out of `enrich.CategoryValues` avoids touching the enrichment contract, the `jobview` category override, and the generated contracts, and it decouples "is this technical?" from "which technical role?". Alternative considered — a single `non_tech` catch-all category or granular non-tech categories (healthcare/hospitality/…) — rejected for now: both widen the contract and the `category` facet for a question a boolean answers.

**2. Tri-state `is_tech` (`*bool`), tech-wins precedence.**
In `jobderive.Derive`: `true` if the derived category is a recognized technical category; else `false` if the category ∈ `NonTechCategories` OR `classify.IsNonTech(title)`; else `nil`. Rationale: technical evidence (the title dictionary) is checked first and always wins, so a non-tech noun never overrides a real tech role ("Sales Engineer" already resolves a tech category). `nil` for unknown is deliberate — coercing unknown to `true` (the current enrichment-gate blacklist logic) would hide exactly the mass we want to measure and shrink. Alternative — strict boolean, unknown→true — rejected: it makes the 69% invisible.

Which categories count as "technical" = `CategoryValues` minus `NonTechCategories` minus `other`. Expose this partition as `enrich.TechCategories` (or a helper) so the derivation reads from one source of truth, guarded by a test that the three sets partition `CategoryValues`.

**3. DB boolean nullable; wire + facet as a string enum.**
`jobs.is_tech boolean` (nullable) — natural for SQL measurement (`is_tech IS TRUE/FALSE/NULL`). `jobview` maps the `*bool` to a string enum `is_tech: "tech" | "non_tech"` (omitted when unknown), because the whole facet stack (Meili facetDistribution, `search/facets.go`, `query_filter`, the FilterModal) is string-value oriented and `roles`/`work_mode` set the precedent for an index-time string facet with `IS EMPTY` for the absent bucket. Meili boolean faceting is avoided as less predictable than string faceting.

**4. Persisted + backfilled like the other dict facets.**
`is_tech` flows through `job.New` → `UpsertJob` (new column in the upsert) and is re-derived by `cmd/backfill-derive` (which already re-derives every dict facet in one pass). No new worker.

## Risks / Trade-offs

- **False-positive: a tech job flagged non-tech** → Mitigated by tech-wins precedence (title dictionary checked first) and by curating `nonTechTable` with unambiguous role nouns only (no bare "engineer"/"technician"/"analyst"); bias is to under-detect (leave in unknown) rather than mislabel. A unit test locks representative tech titles to `is_tech != false`.
- **New filterable attribute 500s until reindex** → Meili rejects a filter on an attribute not yet in the index settings. Mitigated by the standard order: deploy the new binary, run `make reindex` (rebuilds settings + docs) before the UI filter is used. Documented in the migration plan.
- **Unknown bucket still large after title-only detection** → Accepted: this change is measurement-first. The `nil` state quantifies the remaining gap; description-based detection is the noted next slice.
- **Backfill + reindex cost at 3M jobs** → Same cost profile as any dict-facet change; run backfill-derive then reindex off-peak (per the reindex freeze / cron-stacking notes).

## Migration Plan

1. Apply migration `jobs.is_tech boolean` to prod manually **before** deploy (per the migrations gotcha — initdb does not re-run).
2. Deploy the new binary (ingest now persists `is_tech`; search settings include the new filterable attribute).
3. Run `cmd/backfill-derive` to populate `is_tech` on existing jobs, then `make reindex` so the facet/filter is live (reindex must follow backfill and precede UI use of the filter).
4. Measure: `SELECT is_tech, count(*) FROM jobs WHERE closed_at IS NULL GROUP BY 1` — decide facet-only vs catalogue exclusion on the real split.

Rollback: the column and detector are additive; reverting the binary leaves `is_tech` unread. No data migration to undo.

## Open Questions

- None blocking. The choice between facet-only and catalogue exclusion is intentionally deferred to post-measurement, not part of this change.
