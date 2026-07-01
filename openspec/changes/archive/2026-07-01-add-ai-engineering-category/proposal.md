## Why

The catalogue conflates two distinct roles under one `ml_ai` category: classic
ML/DL model work (training, fine-tuning, computer vision) and the emerging
"AI Engineer" role (RAG, agents, LLM applications, prompt engineering). These
are different jobs with different skills and audiences — a field analysis of 895
AI-Engineer postings (builtin.com, Jan 2026) found ~70% are LLM-application
roles that have little to do with model training. Folding them into one facet
hides a signal users want to filter on.

## What Changes

- Add a new role category `ai_engineering` (label "AI Engineer") to the
  controlled vocabulary, alongside the existing `ml_ai`.
- Split the title-classification dictionary so LLM/GenAI/agent/RAG/prompt/applied-AI
  titles resolve to `ai_engineering`, while classic ML/DL titles
  (`machine learning`, `deep learning`, `ml engineer`) and explicitly mixed
  ML-carrying titles (`ml/ai`, `ai/ml`) stay `ml_ai`.
- The LLM enrichment category enum extends automatically (it reads the same
  vocabulary), so the model may also emit `ai_engineering`.
- The web category filter gains the "AI Engineer" option (regenerated contracts +
  label).

No breaking change: `ml_ai` is unchanged in name; `ai_engineering` is purely
additive to the enum. Existing jobs re-tag on the next dictionary re-derive.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `deterministic-facets`: the `category` facet vocabulary gains `ai_engineering`,
  and the title-dictionary classification rule that currently routes all AI/ML
  titles to `ml_ai` is split — AI-application titles resolve to `ai_engineering`,
  classic-ML and mixed ML-carrying titles remain `ml_ai`.

## Impact

- `internal/enrich/enrichment.go` — `CategoryValues` gains `ai_engineering`
  (auto-extends the langchaingo enrichment enum in `internal/enrich/langchain.go`).
- `internal/classify/dictionaries.go` — `categoryOrder` / `categoryAliases`
  re-routed and re-ordered (ML-carrying multiword aliases precede bare AI terms).
- `internal/classify/classify_test.go` — existing AI-title expectations updated,
  mixed-title boundary cases added.
- Web SPA category filter — regenerated contracts + "AI Engineer" label.
- No DB migration (`jobs.category` is a text column, not a PG enum).
- Operational (post-deploy, per the dict-facet convention): `cmd/backfill-derive`
  then `make reindex` to re-tag and re-index existing jobs.
