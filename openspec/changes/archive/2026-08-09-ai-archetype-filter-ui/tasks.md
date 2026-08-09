## 1. Backend: generated values enum

- [x] 1.1 Add `AIArchetypeValues []string` to `internal/vocab` (the six
      literal archetype slugs)
- [x] 1.2 Unit test cross-checking `vocab.AIArchetypeValues` against
      `internal/aiarchetype`'s rule-table archetype slugs, so the two cannot
      drift apart
- [x] 1.3 Add one `emitVocab(...)` line to `cmd/gen-contracts/main.go`'s
      `genVocab()` for `AI_ARCHETYPE_VALUES` (mirroring the
      `CATEGORY_VALUES`/`SENIORITY_VALUES` lines)
- [x] 1.4 Run `make gen-contracts`; verify `AI_ARCHETYPE_VALUES` appears in
      `web/src/lib/generated/contracts.ts`

## 2. Frontend: labels and facet registration

- [x] 2.1 Add `AI_ARCHETYPE_LABELS` to `web/src/lib/labels.ts` (the six
      hand-written labels: "RAG Application Builder", "Agent Builder",
      "Cloud/ML Platform Engineer", "ML Trainer/Researcher", "Full-Stack AI
      Engineer", "DevOps/Infra Engineer"), matched against the generated
      `AI_ARCHETYPE_VALUES`
- [x] 2.2 Add a static `AI_ARCHETYPE` options array (`options(AI_ARCHETYPE_VALUES,
      AI_ARCHETYPE_LABELS)`) and one new `FACETS` entry — `param:
      'ai_archetype'`, `label: 'AI Specialization'`, `control: 'select'`,
      `options: AI_ARCHETYPE`, `excludable: true` — positioned immediately
      after the `category` entry in `web/src/lib/facets.ts`
- [x] 2.3 Wire the facet into the actual UI: a `RailEntry` in
      `web/src/lib/filterSections.ts` (`RAIL`, right after `category`,
      `kind: 'facet'`) so the modal renders it; a live-count merge fix in
      `web/src/lib/components/facets/FacetSection.svelte`'s static-select
      branch (it previously only merged counts for `dynamic: true` facets —
      a generic fix, not ai_archetype-special-cased); a
      `facetGroup('ai_archetype', ...)` line in `FilterSummary.svelte` so an
      applied selection shows as a sidebar chip. Registering a `FacetDef` in
      `FACETS` alone is necessary but not sufficient — `RAIL` and
      `FilterSummary` are separate hand-maintained lists

## 3. Verification

- [x] 3.1 `go build ./...`, `go vet ./...`, `go test ./...` (backend) — clean
- [x] 3.2 Web type-check / lint per `web/AGENTS.md` conventions — `svelte-check`
      0 errors, `vitest run` 79 files / 862 tests passing, `oxlint` clean on
      every changed file
- [x] 3.3 Live-browser visual check attempted but not completed: the only
      free local Postgres/Meilisearch belonged to a concurrent worktree
      (`tailor-experience-tab`) whose Meilisearch had no `jobs` index yet
      (empty dev instance, unrelated to this change), so the SSR jobs page
      500s on load regardless of this diff. Seeding data or reindexing that
      shared instance was ruled out to avoid disturbing concurrent work.
      Substituted with the automated tests (`facets.test.ts`,
      `filterSections.test.ts`, `labels.test.ts`) asserting the exact
      behavior a visual check would confirm — position, control type, label
      text, live-count merge — plus a second code-review pass that traced
      the actual `FilterModal.svelte` render path end-to-end. A real
      browser check should still happen post-deploy against a populated
      environment.
