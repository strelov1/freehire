## 1. Section-aware CV parsing (new pure package)

- [ ] 1.1 Add `internal/cvsection` with a `Parse(cvText) (declared, body, all []string)` that heading-splits the CV (Skills section vs body, EN+RU) and skill-tags each segment
- [ ] 1.2 Table-driven tests: declared+body split, skill in both, skill only in body, no Skills heading ⇒ declared empty, determinism

## 2. Enriched market-coverage verdict (backend)

- [ ] 2.1 Add `Facets:["skills"]` to the role facet query in `computeCoverage` and thread the CV-section sets in from the stored CV
- [ ] 2.2 Extend `verdict.Verdict`/`verdict.Compute`: top-20 `SkillRow`s (name, market_frequency, must_have, status, advice), `must_have_total`/`must_have_covered`, `stack_match_percent`, `coherence_percent`; add `MustHavePct` const
- [ ] 2.3 Deterministic status classification (strong/hidden/missing) and status-keyed advice templates
- [ ] 2.4 Table-driven `verdict` tests: status derivation, must-have threshold, stack-match, coherence (incl. empty declared), unchanged coverage headline

## 3. Restructured CV ATS score (backend)

- [ ] 3.1 Replace `atscheck.Report` shape with categories/line-items + `overall`/`potential` + strong/recommended keyword lists; keep `Score` pure
- [ ] 3.2 Map the existing structural checks into Format/Section/Length line items with point attribution; Keyword Strength from the role top-N match
- [ ] 3.3 Content Quality deterministic proxy (action verbs + quantified results) as the no-LLM fallback; `ApplyReview` sets it from the LLM score and re-sums `overall`
- [ ] 3.4 Rename `Review.Findings` → `Suggestions`; update analyzer prompt/sanitize and `PostATSReport`/`GetATSReport` wiring
- [ ] 3.5 `atscheck` tests: category scores, overall=sum, potential, strong/recommended split, proxy vs LLM content-quality, determinism

## 4. Contracts + frontend

- [ ] 4.1 Regenerate TS contracts via `cmd/gen-contracts`; update `web/src/lib/types.ts` re-exports if needed
- [ ] 4.2 Rewrite `VerdictView.svelte`: coverage headline + must-have/stack/coherence stat row + top-20 breakdown with status badges (green/amber/red) and advice
- [ ] 4.3 Rewrite `ATSReportView.svelte`: 5 category cards with per-item attribution, strong-keyword list, recommended-keyword chips, overall + potential, numbered suggestions
- [ ] 4.4 Reconcile `verdict/+page.svelte` glue (review button state, no-CV state) with the new shapes; verify via `svelte-check`

## 5. Verify

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` green
- [ ] 5.2 Manual/visual check of both tabs with and without an LLM configured
