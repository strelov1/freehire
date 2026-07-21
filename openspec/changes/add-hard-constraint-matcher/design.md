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

**D2 — Approach B, fully structured certifications.** Add `required_certifications []string` to the enrichment payload and `Certifications []string` to the résumé structured shape; both are normalized through one shared curated **credential vocabulary** (alias→canonical, IT-first + global) so comparison is slug↔slug. *Why:* consistent with structured-first; avoids brittle text scanning; the vocabulary is still needed either way to make two LLM outputs comparable. *Alternative rejected:* compute-time text scan (approach A) — cheaper rollout but reintroduces the fragility we're trying to leave behind.

**D3 — Severity tiers drive a score-cap ceiling.** Lower cap = harder: `work_auth` 50, `certification` 60, `education`/`experience` 65, `language` 70, `location_workmode` 75. The overall cap is `min(ScoreCap)` over **unmet** blockers; matchanalysis clamps its server-owned `overall_score` to that cap. *Why:* mirrors JobSentinel's proven "a missing required constraint ceilings the score" idea, but enforced where our score is already authoritative. *Alternative rejected:* subtractive penalties — harder to reason about and to test than a ceiling.

**D4 — Suppress the "or equivalent" degree false-positive at the source.** The enrichment prompt is instructed to leave `education_level` unset when the posting says "degree or equivalent experience"; education stays `medium` severity, never `hard`. *Why:* the cleanest place to encode the nuance is the extractor, not a downstream regex patch (JobSentinel's approach). *Trade-off:* relies on prompt adherence; medium severity bounds the blast radius of a miss.

**D5 — Precision rule #1: both-sides-present or skip.** A category is evaluated only when the job carries the requirement AND the résumé carries the evidence field. Missing enrichment field, absent résumé structure, or unparseable value → category silently skipped (no blocker, no cap). Language defaults to info-only because CV `languages[]` rarely encode a CEFR level. *Why:* a false blocker is the worst outcome; graceful degradation is mandatory during the catalogue re-enrich window.

**D6 — Recompute the cap on read, do not cache it.** The deterministic cap is cheap (a handful of comparisons), so matchanalysis recomputes the blockers and the ceiling from the current job + résumé + dictionary on every read/serve and clamps the cached LLM dimensions to that live cap. The cache keeps its existing three stamps (CV upload, job content_hash, model) unchanged. *Why:* the cap can never be stale because it is never persisted, so a vocabulary/ladder change takes effect immediately with **no new stamp and no DB migration** on the analysis cache table (a fourth stamp column would have required one). *Trade-off:* the cached LLM dimensions/gaps still reflect the blockers from when they were generated, so an advisory dimension can lag a dictionary change until the next recompute — acceptable because the authoritative guardrail (the ceiling) is always live. *Alternative rejected:* a fourth `hardconstraint` cache stamp — correct but forces a migration the design set out to avoid, to guard a value cheap enough to just recompute.

## Risks / Trade-offs

- **False blocker erodes trust** → D5 (both-sides-present), conservative severities (education medium, language info-only, location soft), and D4 suppression. When in doubt, skip.
- **Catalogue re-enrich cost/latency** → the field populates on new jobs for free; existing jobs re-enrich in waves via `cmd/enrich`; an empty `required_certifications` just skips the certification category — no wrong output, only reduced coverage until backfilled.
- **Credential vocabulary drift / US-centric bloat** → port trimmed to IT-first + genuinely global credentials; keep it small and curated; recompute-on-read (D6) means a vocabulary update takes effect immediately with no stale caps to invalidate.
- **Prompt non-adherence for "or equivalent" (D4)** → bounded by medium severity; revisit if false education blockers show up in review.
- **jobmatch bar needs new inputs** (enrichment + structured résumé at that handler) → verify availability at the call site; if the bar endpoint lacks the structured résumé, degrade to skill-coverage-only there while matchanalysis still applies the cap.

## Migration Plan

- No DB migration: both new fields live in existing jsonb columns; `required_certifications` stays out of facets (no reindex).
- Ship the package + add-fields + prompt changes; regenerate TS via `cmd/gen-contracts`.
- Populate new jobs automatically; drain existing catalogue via `cmd/enrich` waves post-deploy.
- Résumé field self-heals on next upload (stale structure already reads as absent).
- Rollback: the evaluator is additive; disabling the cap/blocker surfaces reverts to today's behaviour with no data cleanup (the extra jsonb fields are inert).

## Resolved (from task 1.1 exploration)

- **Experience tolerance:** strict — `cv.total_years < job.experience_years_min` is a blocker, no grace year.
- **Job requirement access:** `db.Job` already exposes `ExperienceYearsMin`, `EducationLevel`, `EnglishLevel`, `WorkMode`, `Countries` as columns; `required_certifications` is read by unmarshalling the existing `job.Enrichment` jsonb into `enrich.Enrichment` (no new column).
- **Résumé access:** `resume.Structured(ctx, userID)` provides the structured shape (`total_years`, `education[]`, `certifications`).
- **Profile-match bar wiring:** the `JobMatch` handler currently loads only the skills profile, not the structured résumé — it must additionally load `resume.Structured` (and read the job requirement fields) to attach blockers; absent structure degrades to coverage-only (D5).
- **Cache stamp:** superseded by D6 — the cap is recomputed on read, so no fourth stamp and no migration on the analysis cache table.
