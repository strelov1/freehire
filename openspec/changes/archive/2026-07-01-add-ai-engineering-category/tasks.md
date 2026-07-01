## 1. Vocabulary

- [x] 1.1 Add `ai_engineering` to `enrich.CategoryValues` in `internal/enrich/enrichment.go` (next to `ml_ai`); confirm `go test ./internal/enrich/...` stays green (langchaingo enum auto-extends).

## 2. Title classification (TDD)

- [x] 2.1 RED: in `internal/classify/classify_test.go`, change the existing AI-title cases (GenAI / LLM / Prompt / Applied AI Engineer) to expect `ai_engineering`, and add boundary cases: `Machine Learning Engineer`/`Deep Learning Engineer`/`ML Engineer` → `ml_ai`, `ML/AI Engineer`/`AI/ML Engineer` → `ml_ai`, `RAG Engineer`/`AI Engineer` → `ai_engineering`. Run the test, watch it fail.
- [x] 2.2 GREEN: in `internal/classify/dictionaries.go`, split the AI/ML cluster — route `ai engineer`, `applied ai`, `generative ai`, `genai`, `llm engineer`, `prompt engineer`, `llm` (and add `rag engineer`) to `ai_engineering`; keep `machine learning`, `deep learning`, `ml engineer`, `ml/ai` on `ml_ai` and add `ai/ml` → `ml_ai`. Order `categoryOrder` so the ML-carrying aliases precede the bare AI terms.
- [x] 2.3 Run `go test ./internal/classify/...` — all green, including the canonical-membership assertion.

## 3. Web category filter

- [x] 3.1 Regenerate web contracts with the contract generator (`cmd/gen-contracts`) so `CATEGORY_VALUES` in `web/src/lib/generated/contracts.ts` gains `ai_engineering` (do not hand-edit the generated file).
- [x] 3.2 Add `ai_engineering: 'AI Engineer'` to the `CATEGORY_LABELS` map in `web/src/lib/labels.ts` (next to `ml_ai`); `facets.ts` and `enrichment.ts` both consume this map, so one edit covers both.
- [x] 3.3 Verify the web build (`svelte-check` per the project's web verification — no test runner exists).

## 4. Verify

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` all clean.
- [x] 4.2 Record the post-deploy operational step in the change (run `cmd/backfill-derive` then `make reindex` on prod to re-tag and re-index existing jobs) — code change carries no DB migration.
