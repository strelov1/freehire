## Why

Our fit signal is split between a deterministic skill-coverage bar (`internal/jobmatch`, instant/free) and an LLM match-analysis (`internal/matchanalysis`, opt-in). Neither enforces **hard constraints** — a posting that requires 5+ years, a specific degree, a license/certification, work authorization, or on-site presence can still score high when the caller plainly does not meet it, because skill coverage ignores those axes and the LLM's score drifts run-to-run and cannot be trusted as a guardrail. Both sides of the comparison are already structured (job enrichment carries `experience_years_min`/`education_level`/`english_level`/`visa_sponsorship`/`countries`/`work_mode`; the résumé is parsed into `total_years`/`education[]`/`languages`/`location`), so a cheap, explainable, deterministic checker can surface real blockers and cap an over-optimistic score — no new LLM calls on the hot path.

## What Changes

- Add `internal/hardconstraint` — a pure, deterministic, dict-only package (same discipline as `jobmatch`/`classify`) that evaluates a job's structured requirements against the caller's structured résumé across six categories (experience-years, education, language, work-authorization, location-and-work-mode, certification) and emits typed `Blocker{Category, Severity, ScoreCap, Reason, Action, Met}`.
- Add a shared curated **credential vocabulary** (alias→canonical, IT-first + global; ported/trimmed from JobSentinel) and a **degree ladder / equivalence** dictionary, both under the new package.
- Derive the job's required certifications **deterministically** in `internal/jobfacts` (`RequiredCertifications(description)`, scanning the description with the credential vocabulary) — the same deterministic-facts home as `education_level`/`experience_years_min`/`skills`, computed at read where the evaluator runs, so there is **no enrichment schema change, no LLM re-enrich, and no migration**.
- Add `internal/jobfacts.DegreeOptional(description)` — true when the posting offers a degree "or equivalent experience"; the evaluator skips the education blocker when it is set, suppressing that false positive deterministically at the source (`education_level` keeps its existing precision-tuned meaning for the search facet).
- Add `Certifications []string` to the résumé structured shape (the CV side stays LLM-parsed by `resumeextract`).
- Surface blockers on three surfaces: (1) the instant `jobmatch` bar renders blockers beside skill coverage; (2) `matchanalysis` caps the server-owned `overall_score` by `min(ScoreCap)` over unmet blockers, recomputes that ceiling on read, and injects the known blockers into its Stage-1 and Stage-3 context; (3) `tailor` consumes the anti-hallucination `Action` strings.
- Precision rule #1 — **never a false blocker**: a category is evaluated only when both sides carry data; a missing enrichment field or absent résumé structure skips that category silently. Language is info-only by default (CV languages rarely carry a CEFR level); location/work-mode is soft.
- No behaviour is gated/hidden — blockers warn and cap only; the job stays visible and clickable.

## Capabilities

### New Capabilities
- `hard-constraint-matching`: the deterministic evaluator, `Blocker` model, six requirement categories, severity/score-cap tiers, credential vocabulary, degree ladder, and the "never a false blocker" precision rules.

### Modified Capabilities
- `deterministic-facets`: `internal/jobfacts` gains `RequiredCertifications(description)` (credential-vocabulary scan) and `DegreeOptional(description)` ("or equivalent" detector) — deterministic, computed at read, no stored field.
- `resume-structured-profile`: adds the `certifications` field to the structured résumé shape.
- `job-profile-match`: the profile-match bar additionally surfaces hard-constraint blockers alongside skill coverage.
- `job-fit-analysis`: `overall_score` is capped by the deterministic blockers (recomputed on read), and the blockers are injected into the prompt chain.
- `cv-tailoring`: the tailor flow consumes the blockers' anti-hallucination `Action` strings as guardrails.

## Impact

- **New package:** `internal/hardconstraint` (+ credential/degree dictionaries) and its generated TS `Blocker` wire shape via `cmd/gen-contracts`.
- **No DB migration, no enrichment schema change, no reindex:** the job side (required certifications, degree-optional) is derived deterministically from the description at read; `certifications` on the CV side lives in the existing `users.resume_structured` jsonb (no DDL).
- **Rollout:** the job side needs no backfill — it is computed on read, so it is correct for the whole catalogue the moment the code ships. The résumé `certifications` field self-heals on the next CV upload. Both are additive and degrade gracefully.
- **Touched code:** `internal/jobfacts` (`RequiredCertifications` + `DegreeOptional`), `internal/hardconstraint/credentials` (a `Scan` helper), `internal/resumeextract` (add-field + prompt), the profile-match handler (blocker surface), `internal/matchanalysis` (score-cap + prompt context + recompute-on-read), the tailor context assembly, and the relevant HTTP handlers/wire shapes.
