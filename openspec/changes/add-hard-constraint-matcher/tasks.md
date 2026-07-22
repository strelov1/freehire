## 1. Foundation & dictionaries

- [x] 1.1 Explore call sites: confirm whether the profile-match bar handler already loads the caller's structured résumé + job enrichment, and where `matchanalysis` computes `overall_score` and assembles the Stage-1/Stage-3 prompt and cache stamps. Record findings; resolve the design's open questions (experience tolerance = strict; bar degradation path).
- [x] 1.2 Create `internal/hardconstraint/credentials`: curated credential vocabulary (alias→canonical slug, IT-first + global), ported/trimmed from JobSentinel's credential groups. Provide `Canonical(alias string) (slug string, ok bool)` and the controlled slug set. Table-test alias→canonical + unknown→drop.
- [x] 1.3 Add the degree ladder + equivalence dictionary (none<ged<associate<bachelor<master<phd) with `DegreeRank(name string) (rank int, ok bool)`. Table-test equivalence normalization and ranking.

## 2. Core evaluator (internal/hardconstraint)

- [x] 2.1 Define the wire types in a gen-contracts–visible file: `Blocker{Category, Severity, ScoreCap, Reason, Action, Met}`, the `Category`/`Severity` enums, and the `JobRequirements`/`CVEvidence` input structs. Add `Sanitize`/bounds if any strings are model-derived.
- [x] 2.2 Implement `Evaluate(job, cv) []Blocker` with the experience and education categories first (numeric compare; degree ladder). RED→GREEN table tests: unmet→blocker, met→satisfied, missing-either-side→skipped, strict tolerance.
- [x] 2.3 Add the certification category (canonical-slug intersection via the credential vocabulary) and the language category (info-only by default, never a blocker without a comparable level). Table-test met/unmet/skip.
- [x] 2.4 Add the work-authorization and location-and-work-mode categories (visa_sponsorship + countries vs résumé location / location_preferences; work_mode conflict). Soft severities. Table-test conflict vs geo-uncertainty→skip.
- [x] 2.5 Implement the severity/score-cap tiers and the `min(ScoreCap)` overall-cap helper. Table-test hardest-blocker-wins and no-unmet→no-cap.

## 3. Deterministic job facts + résumé field

- [x] 3.1 Add `credentials.Scan(text) []string` — the canonical slugs whose aliases appear whole-word in text, deduped. Table-test recognized/unrecognized/dedup.
- [x] 3.2 Add `internal/jobfacts.RequiredCertifications(description)` (thin wrapper over `credentials.Scan`) and `internal/jobfacts.DegreeOptional(description)` (regex over "or equivalent experience" / "degree or equivalent" phrasings). Table-test both; a hard requirement → DegreeOptional false.
- [x] 3.3 Add `DegreeOptional bool` to `hardconstraint.JobRequirements`; `appendEducation` skips the blocker when set. Table-test degree-optional → no education blocker.
- [x] 3.4 Résumé: add `Certifications []string` to `resumeextract.Structured` (wire + Sanitize bounds) and to the extraction prompt. Test sanitize bounds; confirm stale/absent degrades.

## 4. Integration surfaces

- [x] 4.1 Profile-match bar: assemble `JobRequirements`/`CVEvidence` at the handler and attach `blockers` to the payload when structured inputs are present; degrade to coverage-only otherwise. Handler/integration test both paths; never hide the job.
- [x] 4.2 matchanalysis: clamp `overall_score` to `min(ScoreCap)` and derive `verdict` from the capped score, applied at serve time. Test the cap scenario (weighted 88 → capped 60).
- [x] 4.3 matchanalysis: inject the blockers into the Stage-1 and Stage-3 prompt context as known constraints; expose blockers in the served analysis. Prompt-content test + analysis-shape test.
- [x] 4.4 matchanalysis: recompute the blockers + ceiling on read (GET) from the current job/résumé/dictionary and apply to the cached dimensions; the cache keeps its existing three stamps unchanged (D6 — no fourth stamp, no migration). Test that a dictionary change re-caps a cached analysis without marking it stale.
- [x] 4.5 Tailor: pass unmet blockers' `Action` strings into the tailor context as explicit "do not claim unless true" guardrails. Test the action strings reach the tailor input.

## 5. Contracts & frontend

- [x] 5.1 Run `cmd/gen-contracts`; regenerate TS for `Blocker`, the new enrichment/résumé fields, and the fit payload. Verify the generated diff is limited to these.
- [x] 5.2 Surface blockers in the profile-match bar UI (advisory warning chips + met ✓), and show the ceiling context on the fit analysis page. Frontend unit test for the pure reducer/formatter; visual check.

## 6. Rollout & verification

- [x] 6.1 Verify no DB migration, no enrichment schema change, and no Meilisearch facet were added (no reindex; job side is compute-at-read). Confirm `go build ./... && go vet ./...` and full `go test ./...` are green.
- [x] 6.2 Confirm no catalogue backfill is required (job side is correct on read the instant the code ships); the résumé `certifications` field self-heals on the next upload. Note this in the change summary.
- [x] 6.3 `verification-before-completion`: drive the fit analysis end-to-end for a caller who misses a hard constraint and confirm the served score is capped and the blocker is surfaced.
