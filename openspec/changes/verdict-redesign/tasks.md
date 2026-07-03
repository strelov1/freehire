## 1. Section-aware CV parsing (new pure package)

- [x] 1.1 Add `internal/cvsection` with a `Parse(cvText) (declared, body, all []string)` that heading-splits the CV (Skills section vs body, EN+RU) and skill-tags each segment
- [x] 1.2 Table-driven tests: declared+body split, skill in both, skill only in body, no Skills heading ⇒ declared empty, determinism

## 2. Enriched market-coverage verdict (backend)

- [x] 2.1 Add `Facets:["skills"]` to the role facet query in `computeCoverage` and thread the CV-section sets in from the stored CV
- [x] 2.2 Extend `verdict.Verdict`/`verdict.Compute`: top-20 `SkillRow`s (name, market_frequency, must_have, status, advice), `must_have_total`/`must_have_covered`, `stack_match_percent`, `coherence_percent`; add `MustHavePct` const
- [x] 2.3 Deterministic status classification (strong/hidden/missing) and status-keyed advice templates
- [x] 2.4 Table-driven `verdict` tests: status derivation, must-have threshold, stack-match, coherence (incl. empty declared), unchanged coverage headline

## 3. Restructured CV ATS score (backend)

- [x] 3.1 Replace `atscheck.Report` shape with categories/line-items + `overall`/`potential` + strong/recommended keyword lists; keep `Score` pure
- [x] 3.2 Map the existing structural checks into Format/Section/Length line items with point attribution; Keyword Strength from the role top-N match
- [x] 3.3 Content Quality deterministic proxy (action verbs + quantified results) as the no-LLM fallback; `ApplyReview` sets it from the LLM score and re-sums `overall`
- [x] 3.4 Rename `Review.Findings` → `Suggestions`; update analyzer prompt/sanitize and `PostATSReport`/`GetATSReport` wiring
- [x] 3.5 `atscheck` tests: category scores, overall=sum, potential, strong/recommended split, proxy vs LLM content-quality, determinism

## 4. Contracts + frontend

- [x] 4.1 Regenerate TS contracts via `cmd/gen-contracts`; update `web/src/lib/types.ts` re-exports if needed
- [x] 4.2 Rewrite `VerdictView.svelte`: coverage headline + must-have/stack/coherence stat row + top-20 breakdown with status badges (green/amber/red) and advice
- [x] 4.3 Rewrite `ATSReportView.svelte`: 5 category cards with per-item attribution, strong-keyword list, recommended-keyword chips, overall + potential, numbered suggestions
- [x] 4.4 Reconcile `verdict/+page.svelte` glue (review button state, no-CV state) with the new shapes; verify via `svelte-check`

## 5. Verify

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green
- [x] 5.2 Visual check of both tabs — verified via a throwaway `_verify` route rendering both components with representative new-contract data + headless-Chrome screenshot (both tabs render correctly with the STRONG/HIDDEN/MISSING status colours, 5 category cards, keyword chips, and suggestions). Full data-path (live LLM + Meili + DB) covered by handler tests rather than a live run.

## 6. Unified profile page (IA + inline editing)

- [ ] 6.1 New `ProfileForm.svelte`: inline single-profile editor — specializations via `SearchSelect` (≤5), skills via `RemoteSearchSelect`, CV drop-zone that shows an "uploaded · update" state when `has_cv`; Save via `profileStore.save`; `canSubmit` = ≥1 specialization & ≥1 skill
- [ ] 6.2 Rewrite `my/profile/+page.svelte` to the `/jobs` layout: filter sidebar (`FilterStore`/`FilterSummary`/`FilterModal`/`FilterEdgeTab`, `skills` excluded) + main column = `ProfileForm` + Market-coverage/CV-readiness tabs; fold in the verdict page's init/reload/runReview; keep the delete button, drop the edit + sparkles buttons
- [ ] 6.3 Filter drives the comparison role independently of the saved profile (category seeded from specializations; changing it never mutates the profile); re-fetch verdict + ATS after a save/upload so the numbers track edits, and drive `ProfileForm`'s CV state from `has_cv`
- [ ] 6.4 Delete `my/profile/verdict/+page.svelte` and `ProfileEditModal.svelte`; drop the `verdictHref` route reference
- [ ] 6.5 Verify: `svelte-check` clean + `vite build` green
