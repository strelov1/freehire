## 1. Backend — prompt budget and safety ceiling

- [x] 1.1 Raise `maxRecommendRunes` to 1600 in `internal/matchanalysis/matchanalysis.go`
- [x] 1.2 Update `stage2SystemPrompt` and `stage3SystemPrompt` with the shared recommendation length/shape contract (2–3 short prose paragraphs, no headings/lists, no requirement-table recap)
- [x] 1.3 Extend `prompt_test.go` so both Stage 2 and Stage 3 prompts assert the recommendation budget language
- [x] 1.4 Add/extend a sanitize unit test that a recommendation under the budget is kept intact and one over the ceiling is truncated

## 2. Frontend — shared markdown helper and verdict card

- [x] 2.1 Promote `web/src/lib/assistant/markdown.ts` (+ `markdown.test.ts`) to `web/src/lib/markdown.ts` / `markdown.test.ts` and update assistant imports
- [x] 2.2 Render the verdict card in `MatchAnalysisFull.svelte` via `renderMarkdown` (wrapper, not a single `<p>`), with `text-base` when `stacked`
- [x] 2.3 Run the markdown unit tests and confirm assistant chat still imports the promoted module

## 3. Verify

- [x] 3.1 `go test ./internal/matchanalysis/` and `go vet -tags=integration ./...`
- [x] 3.2 Frontend unit tests for the markdown module (vitest)
