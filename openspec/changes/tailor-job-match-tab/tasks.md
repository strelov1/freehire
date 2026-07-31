## 1. The scorer — `internal/cvmatch`

- [x] 1.1 Declare the package's wire shape: `LineItem`, `Category` (id, label, earned, weight, available, reason, items), `Score` (overall, categories, contributing category ids), and the four category-id constants with their weights (40/30/20/10). Table-test the JSON tags.
- [x] 1.2 Implement the earned÷possible overall: unavailable categories leave both numerator and denominator; all-four-available yields a plain sum out of 100; an unavailable category scores strictly higher than that category scored zero at full weight.
- [x] 1.3 Implement Keyword Match (30): vacancy canonical skills vs the document's parsed skills, reporting matched and missing skills by name; a vacancy with no canonical skills makes the category unavailable.
- [x] 1.4 Implement Job Title Match (20): 12 points for the vacancy's normalized title occurring in the normalized document text, 8 for the vacancy's `classify.Parse` category appearing in `classify.Categories(cvText)`; an empty vacancy title makes the category unavailable.
- [x] 1.5 Implement Seniority Fit (10): `classify.Parse` on both sides, distance over `vocab.SeniorityValues` (0 → full, 1 → half, ≥2 → none); either side resolving to `""` makes the category unavailable rather than a mismatch.
- [x] 1.6 Implement Requirements Coverage (40): tag each cached requirement's text with `skilltag.Parse`; skill-bearing requirements are covered only when every named skill is present, weighted 2 for `required` and 1 for `preferred`; requirements yielding no canonical skill are unverifiable, carry their cached status labelled, and leave the denominator; an all-unverifiable ledger makes the category unavailable.
- [x] 1.7 Assemble `Score(cvText string, cvSkills, jobSkills []string, jobTitle string, reqs []Requirement) Score` — pure, I/O-free, no LLM — and cover the degraded combinations end to end.

## 2. The endpoint — `GET /me/cvs/:id/job-match`

- [x] 2.1 Split `cvHandlers.scoreRenderedCV` into a shared `renderedCVText(ctx, doc, tmpl) (string, error)` carrying the missing-toolchain check, and re-point the ATS-delta handler at it with its tests still green.
- [x] 2.2 Add the handler: owner-scoped read, 409 naming which case when the CV is the base copy or its vacancy was pruned, and the `{available, reason?, score?}` envelope.
- [x] 2.3 Read the requirement ledger through the existing `cachedAnalysisCtx`; a pair with no cached analysis degrades Requirements Coverage to unavailable rather than failing the request.
- [x] 2.4 Degrade to `available: false` with a reason and a success status when the renderer or the text extractor is missing, or a render fails.
- [x] 2.5 Register the route under `mw.cookie` beside the ats-delta route, and cover the endpoint with an integration test in the `cv_ats_delta` harness's style.
- [x] 2.6 Generate the TS contract via `cmd/gen-contracts` and verify the shape lands in `web/src/lib/generated/contracts.ts`.

## 3. Line items reach the Score tab

- [x] 3.1 Add the tailored side's `Items` to `atscheck.CategoryChange` and carry them through `Compare`; the base side's items are not carried. Existing delta tests stay green.
- [x] 3.2 Extend `viewAtsDelta` in `web/src/lib/tailor/atsdelta.ts` to surface each row's line items, keeping the signed-number and regression wording under vitest.

## 4. The panel

- [x] 4.1 Add `web/src/lib/tailor/jobmatch.ts`: the view model (overall arithmetic as displayed, impact label thresholds from the response's weights, unavailable-category wording) with vitest coverage — no wording in the component.
- [x] 4.2 Add `ScoreCategoryRow.svelte`: one disclosure rendering a category's label, impact, score and expandable line items, driven by props both scorers can satisfy.
- [x] 4.3 Add `JobMatch.svelte`: overall score, the vacancy it was scored against, the four category rows, named missing keywords, and the requirement ledger with unverifiable entries labelled. An unavailable score renders nothing.
- [x] 4.4 Rework `AtsDelta.svelte` to render its rows through `ScoreCategoryRow`, preserving the regression warning and the "measured on the rendered PDF" footnote.
- [x] 4.5 Split `ArtifactPanel`'s tabs to `templates | jd | jobmatch | score`: Job Match carries `JobMatch` plus the labelled `MatchAnalysisFull` snapshot; Score carries `AtsDelta` plus `AutopilotReport`.
- [x] 4.6 Add the View Job link to the panel header, linking to `resolve('/jobs/[slug]', { slug })` and omitted when there is no job.
- [x] 4.7 Add collapse toggles to both side panels: the left panel and the right context panel each fold to a thin rail with a button that restores them, so the centre CV preview can take the full width. Desktop-only (below `lg` the columns are already one at a time), and the collapsed state must not be reachable in a way that hides the panel's tab bar with no way back.

## 5. The workspace wiring

- [x] 5.1 Extend `MobileView` and the mobile tab bar to the eight views, keeping `pickMobile`'s mobile→column sync correct for the two new tabs.
- [x] 5.2 Fetch the job-match score on workspace open and after an agent turn, alongside the existing ATS-delta refresh, never awaited and never throwing.
- [x] 5.3 Refresh the score after `persist()` succeeds — chained off the save, not off the effect that schedules it — so the number never describes the previous document.

## 6. Finish

- [ ] 6.1 `go build ./... && go vet ./... && go test ./...`, `pnpm test` in `web/`, and the integration tag for the new endpoint.
- [ ] 6.2 Visually verify the workspace at desktop and below `lg`: all four right-hand tabs, an expanded category row, and an unavailable score rendering as an absence.
- [ ] 6.3 Update `internal/cv/AGENTS.md` (or add `internal/cvmatch/AGENTS.md`) and the root AGENTS.md module table with the new package.
