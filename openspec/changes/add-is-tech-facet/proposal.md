## Why

freehire is an IT job aggregator, but generic ATS boards (Oracle, Workday, UKG, iCIMS…) pour a company's entire job board into the catalogue — nurses, cleaners, warehouse, retail, hospitality. On prod, of ~3.07M open jobs only 9.5% carry a recognized tech category, 21.5% are confidently non-tech (the 4-category blacklist), and **69% have an empty category** — a mix where at least ~0.5M titles match obvious non-tech nouns. There is no way for a user (or the pipeline) to tell tech from non-tech across that empty mass, and the current `category` facet cannot express it.

## What Changes

- Add a deterministic, confident **non-tech title detector** (`classify.IsNonTech`) — a curated whole-word dictionary of unambiguous non-tech role nouns (registered nurse, forklift operator, warehouse, cashier, housekeeping, electrician, teacher…), same "never guess" doctrine as the existing `classify`/`location`/`skilltag` dictionaries.
- Derive a **tri-state `is_tech` signal** (`*bool`: `true` recognized-tech / `false` confident-non-tech / `nil` unknown) in `jobderive`, tech-category-wins precedence, persisted on `jobs.is_tech` and re-derived by `cmd/backfill-derive`.
- Expose `is_tech` as a **search facet with a filter**: a Meilisearch filterable attribute (`nil` → absent, filterable via IS EMPTY), facet counts on the jobs search, a `jobview` field, and a Tech / Non-tech control in the web FilterModal.

Out of scope (deliberate, later slices): catalog/index exclusion of non-tech, gating enrich/embed on `is_tech`, description-based non-tech detection.

## Capabilities

### New Capabilities
- `tech-classification`: deterministic non-tech title detection, the tri-state `is_tech` derivation and persistence, and its exposure as a filterable search facet.

### Modified Capabilities
<!-- none: is_tech is additive; existing category/facet requirements are unchanged -->

## Impact

- **Code:** `internal/classify` (new non-tech dictionary + `IsNonTech`), `internal/jobderive` (derive `IsTech`), `internal/job` (aggregate field), `internal/jobview` (served field), `internal/search` (filterable attribute + facet + `FromJob` doc), `web/src/lib/facets.ts` + labels (FilterModal control), generated contracts.
- **DB:** new migration `jobs.is_tech boolean` (nullable); `UpsertJob` + backfill-derive queries (`make sqlc`). Apply migration to prod manually before deploy (per the migrations gotcha).
- **Ops:** after deploy, run `cmd/backfill-derive` + `make reindex` to reach existing jobs; then measure the `true/false/null` split on prod.
