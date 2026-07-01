## Context

Role category is a deterministic, dict-only facet. `internal/classify.Parse`
matches a lowercased job title against an ordered alias list
(`categoryOrder`/`categoryAliases` in `internal/classify/dictionaries.go`),
first-match-wins, with whole-word boundaries (`containsWord`). The resolved
canonical comes from `enrich.CategoryValues` and is stored in `jobs.category`,
served dict-only as a Meilisearch facet. Today every AI/ML title — classic ML
and LLM-application alike — collapses to `ml_ai`.

The web SPA renders the category filter from generated contracts; the LLM
enrichment schema enum is generated from the same `CategoryValues`.

## Goals / Non-Goals

**Goals:**
- A distinct `ai_engineering` category (label "AI Engineer") separating
  LLM-application roles from classic ML.
- Title classification routes AI-application vs classic-ML vs mixed titles
  correctly and deterministically.
- The new value flows automatically into the enrichment enum and the web filter.

**Non-Goals:**
- No description-body classification for category (title-only, as today).
- No handling of AI-infra/support titles ("AI Platform/Infrastructure Engineer")
  — they are unclassified by title today and stay so.
- No new source adapter (builtin.com spike returned PARTIAL — redundant with ATS
  coverage; out of scope here).
- No backfill code change — the existing `cmd/backfill-derive` re-derives.

## Decisions

**1. Separate canonical `ai_engineering`, not a rename of `ml_ai`.**
Rationale: the two roles are genuinely distinct; keeping `ml_ai` for model work
preserves history and existing tags. Alternative (rename whole `ml_ai`) loses the
ML-vs-AI-app distinction the change exists to create.

**2. Ordering encodes precedence: ML-carrying multiword aliases before bare AI
terms.** `containsWord` is substring-with-boundaries, so `"ML/AI Engineer"`
contains the substring `ai engineer`. To keep mixed titles in `ml_ai`, the
ML-carrying aliases (`ml/ai`, `ai/ml`, `ml engineer`, `machine learning`,
`deep learning`) are listed in `categoryOrder` ahead of the AI-application
aliases (`ai engineer`, `applied ai`, `generative ai`, `genai`, `llm engineer`,
`prompt engineer`, `llm`). First-match-wins then yields `ml_ai` for mixed titles
and `ai_engineering` for pure AI-application titles. Alternative (regex / token
classifier) is over-engineering for a curated dictionary.

**3. `ai/ml` added as an explicit alias.** Current code has `ml/ai`→ml_ai but not
`ai/ml`; the latter is caught incidentally by `ml engineer` only when "engineer"
follows. Adding `ai/ml`→ml_ai makes the mixed-form intent explicit and robust to
titles like "AI/ML Specialist".

**4. Enrichment enum and web contracts are derived, not hand-edited.** Adding to
`CategoryValues` extends the langchaingo enum automatically; the web filter label
"AI Engineer" is added where category labels live and contracts are regenerated.

## Risks / Trade-offs

- [Title-only classification misroutes the ~28% AI-infra/support "AI Engineer"
  postings the field guide flagged] → Accepted: this matches the existing
  title-only heuristic for all categories; description-level disambiguation is a
  separate, larger effort and a non-goal here.
- [Existing `ml_ai`-tagged jobs that are really AI-engineering do not move until
  re-derive] → Mitigation: run `cmd/backfill-derive` + `make reindex` post-deploy,
  per the established dict-facet operational convention. No code risk; purely an
  ops step.
- [A stale Meili index could 500 on a new value] → Not applicable: `category` is
  an already-filterable attribute; only the value set grows, no new attribute.

## Migration Plan

1. Ship code (vocabulary + dictionary + tests + web label/contracts).
2. Post-deploy: `cmd/backfill-derive` re-derives all facet columns (re-tags
   AI-engineering jobs), then `make reindex` rebuilds the search index.
3. Rollback: revert the commit; `ai_engineering` tags become inert (no consumer
   breaks on an unknown facet value), and the next re-derive restores `ml_ai`.

## Open Questions

None — naming (`ai_engineering` / "AI Engineer") and scope are settled.
