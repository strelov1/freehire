# internal/dict/skilltag — Skill Tagging Dictionary

Deterministic skill tagging feeds a Meilisearch facet (`jobs.skills` text[], not enrichment JSONB).

## Design

- **Curated alias→canonical dictionary**, not NLP. Resolves known aliases, emits nothing for unknowns (**never guesses**). Same design as `internal/dict/location`.
- Parses job description at ingest, persists to `jobs.skills` (text array of canonical skill names). Stored beside, not inside, the `enrichment` JSONB blob — `cmd/enrich` never clobbers it.
- `jobview.FromRow` serves `jobs.skills` directly as the top-level `skills` field (**dict-only** — enrichment.skills excluded from served facet, kept raw in JSONB).
- Indexed as a Meilisearch facet; dictionary change needs `cmd/backfill-derive` + `cmd/reindex` to reach existing jobs.

## The three sides of a canonical

A canonical slug carries three things, all owned here because the canonical set is owned
here, and none derivable from the others:

| Side | Where | What it answers |
|---|---|---|
| the slug | `dictionaries.go` | what text resolves to it (`k8s` → `kubernetes`) |
| the label | `labels.go` | how it is written for a reader (`ci-cd` → `CI/CD`) |
| the description | `descriptions.tsv` | what the thing *is* (`dbt` → a SQL transformation tool) |

`Aliases(canonical)` (`aliases.go`) inverts every alias tier at once — the glossary's
"also written as". It deliberately flattens tiers that do NOT resolve under the same
conditions (a résumé acronym is not accepted on job text): it answers "what else is this
called?", not "may I resolve this here?". Anything asking the second question goes
through `Parse`.

### The description dictionary

- A TSV, not a Go map: the vocabulary is 863 canonicals and each value is a sentence,
  so one unquoted line per skill is what a reviewer can actually read a wave of.
  `internal/dict/location` ships `cities1000.tsv` the same way. **The file itself holds
  far fewer rows than that** — see the ratchet below.
- **The loader is strict** where location's is tolerant — a malformed row fails the
  build rather than being skipped. That file is GeoNames' output; this one is ours, so a
  bad row is a mistake in this repository.
- `cmd/gen-skill-descriptions` drafts a wave with an LLM and **prints** it. A human edits
  and commits. Nothing is generated at request time, and no serving path imports an LLM
  client.
- Coverage is a **ratchet**: `describedFloor` records how many canonicals are described
  and only ever rises. When it reaches the whole vocabulary it is deleted and the test
  becomes the absolute rule the labels carry — a canonical with no description fails the
  build.
- An undescribed skill reads as `""` everywhere, exactly like a slug that is not a skill.
  Surfaces render a described skill differently from an undescribed one, so the absence
  has to be testable and never a placeholder.

## Convention

- `internal/dict/skilltag/` owns the dictionary parser and canonical vocabulary.
- Adding a skill: add an alias→canonical mapping in the dictionary source. The canonical set is owned by `internal/dict/skilltag` alone — skills are a free source fact with no controlled `enrich` vocabulary (unlike regions/seniority/category, there is no `enrich.*Values` for skills).
- No LLM dependency — instant, deterministic.
