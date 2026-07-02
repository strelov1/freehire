## 1. Backend deterministic core (`internal/verdict`)

- [x] 1.1 Write table tests for `verdict.Compute`: demand sort + slug tiebreak, top-20 truncation, fewer-than-20 denominator, `StackMatch` rounding, `Unlock` = count/total (gaps only, none on covered), must-have share boundary at `MustHaveShare`, empty candidate, `Total == 0` (no divide-by-zero)
- [x] 1.2 Implement `Verdict`/`Skill`/`MarketSkills` types + `Compute` to pass 1.1
- [x] 1.3 Add `Verdict.MustHaveGaps()` (must-have gaps in demand order) with tests

## 2. Backend LLM analyzer (`internal/verdict/coherence.go`)

- [x] 2.1 Write tests (using `llm.NewWithModel` with a fake `llms.Model`): valid JSON → clamped coherence + gap-filtered advice; out-of-range coherence clamped to 0-100; advice for non-gap slugs dropped; over-length advice truncated; malformed JSON → error; `NewAnalyzer(nil).Analyze` → `(nil, nil)`; prompt includes gap slugs and truncates résumé at `maxResumeRunes`
- [x] 2.2 Implement `Analyzer`, `Analysis`, prompt build, parse, clamp/sanitize, and nil-client degradation to pass 2.1

## 3. Persistence (`search_profiles.resume_analysis`)

- [x] 3.1 Add migration `migrations/00NN_search_profiles_resume_analysis.sql`: nullable `resume_analysis JSONB`, with rollback comment
- [x] 3.2 Add sqlc queries: `GetSearchProfile` (:one, by id + user_id) and `SetSearchProfileResumeAnalysis` (:exec, scoped by user_id); include `resume_analysis` in the profile row projections; run `make sqlc` and commit generated code
- [x] 3.3 Extend the `searchprofile` service + repository with fetch-one and set-analysis use cases (ownership-scoped) with tests

## 4. Config + server LLM wiring

- [x] 4.1 Add `LLMBaseURL`/`LLMAPIKey`/`LLMModel` to the server config + `Load` (optional; empty disables) with a test
- [x] 4.2 In `cmd/server`, build an `*llm.Client` only when all three are set; pass into `handler.Config`; in `handler.Register` wrap it via `verdict.NewAnalyzer` (nil-safe)

## 5. Handler endpoints (`/api/v1/me/profiles/:id/verdict`)

- [x] 5.1 Reuse résumé text parsing from `resume.go` (share `resumeText`/`pdfText`) for the POST path
- [x] 5.2 Implement `GET .../verdict`: load owned profile (404 otherwise), 503 when facets unconfigured, build category filter, `FacetCounts` → `MarketSkills`, `verdict.Compute` from profile skills, merge stored `resume_analysis`; tests with fake `facetCounter` + fake repo
- [x] 5.3 Implement `POST .../verdict`: parse résumé (400 on empty/invalid), compute deterministic gaps, run analyzer, persist derived analysis, return full verdict; tests for degradation (nil analyzer / LLM error → 200 no coherence), ownership 404, empty text 400
- [x] 5.4 Wire both routes in `handler.Register` next to the other `/me/profiles` routes (cookie-only `RequireAuth`)

## 6. Contracts

- [x] 6.1 Add `internal/verdict` to `cmd/gen-contracts`; run `make gen-contracts`; confirm `Verdict`/`Skill` appear in the committed `web/src/lib/generated/contracts.ts` and are re-exported for `api.ts`

## 7. Frontend

- [x] 7.1 Refactor `VerdictView.svelte` from mock data to a `verdict: Verdict` prop; hide the coherence card when `coherence == null`; humanize role via `categoryLabel`; drop the mock `kind` field
- [x] 7.2 Add `api.ts` calls: `getProfileVerdict(id)` and `analyzeProfileResume(id, input: File|string)` (multipart vs JSON dispatch, mirroring `extractResumeSkills`)
- [x] 7.3 Add route `web/src/routes/my/profiles/[id]/verdict/+page.svelte`: load the verdict, render `VerdictView`, and offer a résumé dropzone (reuse the `ProfileForm` dropzone pattern) that calls `analyzeProfileResume`; `noindex`, auth-gated
- [x] 7.4 Add a "Verdict" link to each profile card in `SearchProfilesView.svelte`
- [x] 7.5 Remove the throwaway `web/src/routes/verdict-preview/+page.svelte`
- [x] 7.6 `svelte-check` clean for the touched files

## 8. Verification

- [x] 8.1 `go build ./... && go vet ./... && go test ./...`; confirm `make sqlc` + `make gen-contracts` output is committed. NOTE: `MustHaveShare` left at the documented default 0.40 — live-facet calibration is an ops follow-up (needs prod facet counts; flagged in design.md Open Questions).
- [x] 8.2 Degradation/ownership/503 paths covered by `resume_verdict_test.go` (GET deterministic, POST with/without LLM, LLM error, 404, empty-text 400); `npm run check` (0 errors) and `npm run build` pass. Live server smoke (Postgres+Meili+LLM) remains a pre-deploy ops step.
