# internal/classify — Seniority & Category Tagging

Deterministic seniority/category tagging from job title, feeding enrichment facets.

## Design

- Parses the **job title** at ingest into canonical `jobs.seniority`/`jobs.category` columns.
- Values from `enrich.SeniorityValues`/`enrich.CategoryValues` — EN+RU aliases, whole-word matched. Russian forms listed as full surface forms (not stems) since matcher requires word boundaries. **Never guesses**.
- Same alias→canonical dictionary design as `internal/location` and `internal/skilltag`.

## Serving: dict-only

`jobview.FromRow` overwrites the nested `enrichment.seniority`/`enrichment.category` with the `jobs` column — the dictionary always wins, the LLM's value is never a fallback. They remain **nested under `enrichment`** so existing search facets, SPA, and generated contracts are unchanged.

## Convention

- Adding a value: add it to `enrich.SeniorityValues`/`enrich.CategoryValues` and the title-matching dictionary.
- Dictionary change needs `cmd/backfill-derive` + `cmd/reindex` to reach existing jobs (same caveat as geography/skills).
