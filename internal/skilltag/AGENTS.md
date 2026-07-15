# internal/skilltag — Skill Tagging Dictionary

Deterministic skill tagging feeds a Meilisearch facet (`jobs.skills` text[], not enrichment JSONB).

## Design

- **Curated alias→canonical dictionary**, not NLP. Resolves known aliases, emits nothing for unknowns (**never guesses**). Same design as `internal/location`.
- Parses job description at ingest, persists to `jobs.skills` (text array of canonical skill names). Stored beside, not inside, the `enrichment` JSONB blob — `cmd/enrich` never clobbers it.
- `jobview.FromRow` serves `jobs.skills` directly as the top-level `skills` field (**dict-only** — enrichment.skills excluded from served facet, kept raw in JSONB).
- Indexed as a Meilisearch facet; dictionary change needs `cmd/backfill-derive` + `cmd/reindex` to reach existing jobs.

## Convention

- `internal/skilltag/` owns the dictionary parser and canonical vocabulary.
- Adding a skill: add an alias→canonical mapping in the dictionary source. The canonical set is owned by `internal/skilltag` alone — skills are a free source fact with no controlled `enrich` vocabulary (unlike regions/seniority/category, there is no `enrich.*Values` for skills).
- No LLM dependency — instant, deterministic.
