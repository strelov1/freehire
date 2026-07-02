## 1. Backend — coverage core (pure)

- [x] 1.1 Reshape `internal/verdict/verdict.go`: replace the top-20 / must-have `Verdict` and `Compute` with the coverage model — `Compute(total, uncoveredTotal int64, uncoveredSkills map[string]int64) Verdict` returning `{Total, Covered, CoveragePercent, Gaps[]}` where each gap is `{Name, NewVacancies, UnlockPercent}`, gaps ranked by `NewVacancies` desc then slug asc, capped at `MaxGaps` (20). Owned-skill exclusion is handled by Query B's filter, so `Compute` needs no candidate arg. Driven with unit tests first (coverage math, rounding, total=0 → 0%, ranking, slug tiebreak, cap).
- [x] 1.2 Delete `internal/verdict/coherence.go` and `coherence_test.go`; drop the `Coherence`/`Advice`/`AnalyzedAt` fields and `MustHaveGaps` from the package. Rewrote `verdict_test.go` to the new shape.

## 2. Backend — search filter assembly

- [x] 2.1 Add a small helper (in `internal/handler/resume_verdict.go` or `internal/search`) that combines the role filter groups from `FilterFromValues` with one AND group per profile skill (`Neq("skills", u)`), producing the single "uncovered" filter via `search.Filter`. Unit-test the combined-filter shape (role groups + NEQ-per-skill groups; empty skills; nil role filter).

## 3. Backend — HTTP endpoint

- [x] 3.1 Rewrite `API.GetResumeVerdict`: build the role filter from the request facet params, defaulting `category` to the profile's specializations when absent; run Query A (`FacetCounts` role → total) and Query B (`FacetCounts` uncovered → uncovered total + distribution); call the new `Compute`; return the coverage `Verdict`. Keep cookie-only + owner-scoped (404) and 503 when `a.facets == nil`. Update `resume_verdict_test.go` (handler tests with a fake facets client) to cover default-role, filter-override, skills-not-a-filter, and 503.
- [x] 3.2 Delete `API.ResumeVerdict` (POST), `resumeVerdictText`, `applyStoredAnalysis`, and `storedAnalysis`; remove the verdict LLM analyzer field/wiring from the API struct and its constructor (leave `internal/llm` and the résumé-storage subsystem used by `ExtractResumeSkills` intact).
- [x] 3.3 In `internal/handler/handler.go` remove the `POST /me/profiles/:id/verdict` route; keep the `GET`. Remove the now-dead verdict analyzer construction in `cmd/server` wiring. Update `resume_storage_test.go` to drop the POST-verdict cases.

## 4. Database

- [x] 4.1 Add migration `migrations/00NN_drop_search_profiles_resume_analysis.sql` with `ALTER TABLE search_profiles DROP COLUMN IF EXISTS resume_analysis;` (include the standard manual-apply header comment).
- [x] 4.2 Remove the `resume_analysis` reads/writes from `internal/db/queries/*.sql` (the `SetResumeAnalysis` query and the column in the profile select); run `make sqlc`; commit generated `internal/db` changes. Fix any references in the searchprofile service.

## 5. Contracts + frontend

- [x] 5.1 Regenerate Go→TS contracts (`cmd/gen-contracts`); update `web/src/lib/types` so `Verdict` is the coverage shape and the coherence/`ResumeStatus`-for-verdict types are gone.
- [x] 5.2 Update `web/src/lib/api`: drop `analyzeProfileResume`, `rerunProfileCoherence`, and the verdict-page résumé-storage calls; keep `getProfileVerdict` (now accepting facet query params) and the profile-form `extractResumeSkills`.
- [x] 5.3 Rewrite `web/src/lib/components/VerdictView.svelte` to render coverage (count + percent) and the ranked gap list (`+N vacancies`, `+X%`); remove the coherence card, scoreboard, must-have and top-20 breakdown.
- [x] 5.4 Rewrite `web/src/routes/my/profiles/[id]/verdict/+page.svelte`: remove all résumé-upload/coherence UI; mount `FiltersPanel` bound to the page query params (URL-synced) and refetch the verdict on change.
- [x] 5.5 Update `web/src/lib/components/SearchProfilesView.svelte` + `web/src/lib/skillGap.ts` so the profile card shows the headline coverage (`covered` count + `coverage_percent`) from the new model, keeping the "missing skills → /jobs" links.

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`; run `svelte-check` and lint (per repo baseline) on `web/`; confirm the verdict page recomputes on filter change and the profile card shows coverage. Note the manual prod migration apply in the finish step.
