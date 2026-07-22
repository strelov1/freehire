## Context

`internal/jobmatch` computes deterministic skill coverage (exact/adjacent/missing) and `internal/matchanalysis` runs a three-stage LLM chain whose `overall_score` is server-owned. Job requirements are already typed in the enrichment payload (`experience_years_min`, `education_level`, `english_level`, `visa_sponsorship`, `countries`, `work_mode`) and the résumé is parsed into a typed `Structured` shape (`total_years`, `education[].degree`, `languages`, `location`). Nothing today reconciles the two on non-skill axes, so a plainly-unqualified match can score high, and the LLM score cannot serve as a guardrail because it drifts between runs.

Constraint: freehire is a job board, not a gatekeeper. A wrong "you're blocked" is worse than a missed one. The design must never emit a false blocker.

## Goals / Non-Goals

**Goals:**
- A pure, deterministic, unit-testable `internal/hardconstraint` evaluator shared by three surfaces (jobmatch bar, matchanalysis, tailor).
- Structured-first: compare typed enrichment fields against the typed résumé; the only new extraction is certifications, added as typed fields on both sides (approach B).
- Explainable output: every blocker carries a human `Reason` and an anti-hallucination `Action`.
- A deterministic ceiling on the server-owned `overall_score` that the LLM cannot override.

**Non-Goals:**
- No hiding, gating, downranking, or filtering of jobs (warn + cap only). A user-facing "hide hard-blocked" filter is explicitly deferred.
- No regex extraction of requirements from raw text (structured-first; the credential vocabulary normalizes structured LLM outputs, it does not scrape).
- No Meilisearch facet for `required_certifications` in v1 (no reindex).
- Not a replacement for the LLM's semantic judgement — a cheap pre-filter and guardrail beside it.

## Decisions

**D1 — Pure package with injected data, no I/O.** `hardconstraint.Evaluate(job JobRequirements, cv CVEvidence) []Blocker` takes plain structs assembled by callers; it performs no DB/LLM access. Same shape as `jobmatch.Compute`. *Why:* trivially table-testable, reusable across all three surfaces, and holds the "never guess" dict-only discipline. *Alternative rejected:* inlining logic in matchanalysis — fails the shared-module goal the design requires.

**D2 — Job side is deterministic (jobfacts), CV side is LLM (resumeextract), one shared vocabulary.** The job's required certifications are derived by `internal/jobfacts.RequiredCertifications(description)` — a credential-vocabulary scan of the description, the same deterministic-facts home as `education_level`/`experience_years_min`/`skills` — computed at read where the evaluator runs. The CV's `certifications` are parsed by the LLM `resumeextract`. Both sides normalize through the one shared credential vocabulary (alias→canonical) so comparison is slug↔slug. *Why:* the codebase already derives every comparable hard-constraint fact deterministically in jobfacts, not via the enrichment LLM (the `education_level` the evaluator reads comes from jobfacts, not the enrichment prompt). Deriving certs the same way is consistent, needs **no enrichment schema change, no LLM re-enrich, and no migration**, and is correct for the whole catalogue the instant it ships (compute-at-read). *Alternatives rejected:* (a) an LLM enrichment `required_certifications` field (the originally-approved "approach B") — inconsistent with how facts are actually derived and forces an expensive catalogue re-enrich; (b) a stored jobfacts column — needs a migration to guard a value cheap enough to recompute on read.

**D3 — Severity tiers drive a score-cap ceiling.** Lower cap = harder: `work_auth` 50, `certification` 60, `education`/`experience` 65, `language` 70, `location_workmode` 75. The overall cap is `min(ScoreCap)` over **unmet** blockers; matchanalysis clamps its server-owned `overall_score` to that cap. *Why:* mirrors JobSentinel's proven "a missing required constraint ceilings the score" idea, but enforced where our score is already authoritative. *Alternative rejected:* subtractive penalties — harder to reason about and to test than a ceiling.

**D4 — Suppress the "or equivalent" degree false-positive deterministically.** `internal/jobfacts.DegreeOptional(description)` returns true when the posting offers a degree "or equivalent experience"; the evaluator skips the education blocker when the caller passes that flag. `education_level` itself is left unchanged (it is precision-tuned and also feeds the search facet), and education stays `medium` severity. *Why:* a deterministic detector is reliable and testable, and lives beside the other jobfacts derivations — cleaner than instructing the enrichment prompt (which does not even extract `education_level`) or patching regex downstream. *Trade-off:* the detector is a small regex over known phrasings; medium severity bounds the blast radius of a missed phrasing.

**D5 — Precision rule #1: both-sides-present or skip.** A category is evaluated only when the job carries the requirement AND the résumé carries the evidence field. Missing enrichment field, absent résumé structure, or unparseable value → category silently skipped (no blocker, no cap). Language defaults to info-only because CV `languages[]` rarely encode a CEFR level. *Why:* a false blocker is the worst outcome; graceful degradation is mandatory during the catalogue re-enrich window.

**D6 — Recompute the cap on read, do not cache it.** The deterministic cap is cheap (a handful of comparisons), so matchanalysis recomputes the blockers and the ceiling from the current job + résumé + dictionary on every read/serve and clamps the cached LLM dimensions to that live cap. The cache keeps its existing three stamps (CV upload, job content_hash, model) unchanged. *Why:* the cap can never be stale because it is never persisted, so a vocabulary/ladder change takes effect immediately with **no new stamp and no DB migration** on the analysis cache table (a fourth stamp column would have required one). *Trade-off:* the cached LLM dimensions/gaps still reflect the blockers from when they were generated, so an advisory dimension can lag a dictionary change until the next recompute — acceptable because the authoritative guardrail (the ceiling) is always live. *Alternative rejected:* a fourth `hardconstraint` cache stamp — correct but forces a migration the design set out to avoid, to guard a value cheap enough to just recompute.

## Risks / Trade-offs

- **False blocker erodes trust** → D5 (both-sides-present), conservative severities (education medium, language info-only, location soft), and D4 suppression. When in doubt, skip.
- **Credential vocabulary drift / US-centric bloat** → port trimmed to IT-first + genuinely global credentials; keep it small and curated. Because both the job scan and the cap are computed on read, a vocabulary update takes effect immediately across the whole catalogue with nothing stale to invalidate.
- **Missed "or equivalent" phrasing (D4)** → bounded by medium severity; the detector covers the common phrasings and can grow if review surfaces a false education blocker.
- **jobmatch bar needs new inputs** (job description + structured résumé at that handler) → the description is already on the job row; the handler must additionally load the structured résumé, and degrades to skill-coverage-only when it is absent while matchanalysis still applies the cap.

## Migration Plan

- No DB migration, no enrichment schema change, no reindex. The job side (required certifications, degree-optional) is computed from the description on read; `certifications` on the CV side lives in the existing `users.resume_structured` jsonb.
- Ship the package + jobfacts derivations + résumé add-field; regenerate TS via `cmd/gen-contracts`.
- No catalogue backfill: the job side is correct for every job the instant the code ships (compute-at-read). The résumé `certifications` field self-heals on the next CV upload (stale structure already reads as absent).
- Rollback: the evaluator is additive; disabling the cap/blocker surfaces reverts to today's behaviour with no data cleanup.

## Resolved (from task 1.1 exploration)

- **Experience tolerance:** strict — `cv.total_years < job.experience_years_min` is a blocker, no grace year.
- **Job requirement access:** `db.Job` already exposes `ExperienceYearsMin`, `EducationLevel`, `EnglishLevel`, `WorkMode`, `Countries` as columns; `required_certifications` is read by unmarshalling the existing `job.Enrichment` jsonb into `enrich.Enrichment` (no new column).
- **Résumé access:** `resume.Structured(ctx, userID)` provides the structured shape (`total_years`, `education[]`, `certifications`).
- **Profile-match bar wiring:** the `JobMatch` handler currently loads only the skills profile, not the structured résumé — it must additionally load `resume.Structured` (and read the job requirement fields) to attach blockers; absent structure degrades to coverage-only (D5).
- **Cache stamp:** superseded by D6 — the cap is recomputed on read, so no fourth stamp and no migration on the analysis cache table.
