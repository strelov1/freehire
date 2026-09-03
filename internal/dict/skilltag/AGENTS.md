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

- A TSV, not a Go map: 863 rows, one per canonical, each value a sentence — one
  unquoted line per skill is what a reviewer can actually read a wave of.
  `internal/dict/location` ships `cities1000.tsv` the same way.
- **The loader is strict** where location's is tolerant — a malformed row fails the
  build rather than being skipped. That file is GeoNames' output; this one is ours, so a
  bad row is a mistake in this repository.
- `cmd/gen-skill-descriptions` drafts a wave with an LLM and **prints** it. A human edits
  and commits. Nothing is generated at request time, and no serving path imports an LLM
  client.
- **Coverage is total, and a test says so**: a canonical with no description fails the
  build, the same absolute rule the labels carry. It got there behind a ratchet — a
  `describedFloor` that only ever rose — because 863 reviewed sentences in one pull
  request is a review nobody performs. The ratchet is gone; adding a skill now means
  adding its description in the same commit.
- `Description` still answers `""` for a slug that is not a canonical — the surfaces
  that render it must not print a placeholder — but no canonical reaches that path any
  more, which is why the chip's reveal is unconditional.

## What a word boundary is, and why it is not ASCII

Every alias here is ASCII, which is exactly what made it tempting to test a boundary
by looking at ASCII bytes — and that is a statement about the TERM, not about its
NEIGHBOURS. Both places a boundary is decided read Unicode:
`wordmatch.TechTermBoundary` (the phrase pass) and `wordTokenRE` (the word pass).

A byte-level test reads any letter outside ASCII as a separator, so a curated alias
becomes a whole word inside every accented or non-Latin one. Measured over 4,800 live
prod descriptions from eight sources (2026-09-03), fixing it removed **89 tags across
74 postings (1.5%) and added none** — widening the class can only ever lengthen a
token, so it removes fragment matches and can never lose a term the text states:

| False tag | Came from |
|---|---|
| `typescript` ×51 | the `ts` alias inside Swedish `möts` |
| `elk` | Hungarian `elkészítése` — 18 of 110 postings on a live Hungarian IT crawl |
| `express` / `nim` / `vue` | Portuguese `expressão`, Catalan `mínim`, French `prévue` |

Some of the removals are second-order and correct: an ambiguous single word (`ai`,
`erp`, `networking`) tags only when a strong match corroborates it, so a posting whose
only strong match was the false `typescript` loses both.

The one thing it gives up: an English tech word inflected with a foreign suffix —
Russian `devopsа` — used to match by the same accident that caused the false
positives. That was 1 posting in 4,800, and the Russian sources lost essentially
nothing, because Russian postings write the term with a space or a hyphen.

## Convention

- `internal/dict/skilltag/` owns the dictionary parser and canonical vocabulary.
- Adding a skill: add an alias→canonical mapping in the dictionary source. The canonical set is owned by `internal/dict/skilltag` alone — skills are a free source fact with no controlled `enrich` vocabulary (unlike regions/seniority/category, there is no `enrich.*Values` for skills).
- No LLM dependency — instant, deterministic.
