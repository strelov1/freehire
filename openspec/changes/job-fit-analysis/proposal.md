## Why

The job page already shows a **Profile match** — but it is a purely deterministic
skills set-operation (exact/adjacent/missing coverage over canonical skilltag slugs).
It never reads the vacancy's full text, the company context, or the candidate's actual
experience, so it cannot answer the question a candidate really has: *would an ATS and a
recruiter consider me a fit for THIS role?* In particular it says nothing about **job-title
alignment** and **relevant experience** — the two signals an ATS keyword-screen and a human
recruiter weigh most heavily.

## What Changes

- Add an on-demand, LLM-driven **fit analysis** for a single (candidate, job) pair. A new
  "AI fit analysis" action under the existing deterministic match bar triggers a **fixed
  three-stage LLM prompt-chain** (not an autonomous agent — deterministic, typed, cacheable)
  that compares the full vacancy (title + description), the company context (`company_info`),
  and the candidate's stored CV text, using the deterministic skills match as a grounding anchor:
  - **Stage 1 — Extract & Match (the ATS lens):** extract the posting's explicit
    requirements + role-title/seniority signals, then classify each against the CV text as
    `covered / synonym-only / missing-but-have / missing-gap` (the reference project's ATS
    keyword-coverage check). This is where "what an ATS keyword-screen sees" lives.
  - **Stage 2 — Recruiter verdict (the human lens):** given Stage 1's structured match +
    `company_info` + CV + the deterministic anchor, produce the five-dimension scored verdict.
  - **Stage 3 — Adversarial audit:** a critic pass that challenges Stage 2 (inflated scores,
    fabricated strengths, glossed-over gaps) and corrects it — the reference's drafter→reviewer
    honesty check.
- The chain returns a **five-dimension** structured verdict — Title & role alignment,
  Experience relevance, Seniority fit, Skills coverage, Company & role context — each scored
  0–100, plus the Stage 1 requirement-match table, a weighted `overall_score`, a `verdict`
  label (Strong/Good/Moderate/Weak/Poor Fit), `strengths[]`, `gaps[]`, and one `recommendation`
  line. Title alignment and experience relevance are the emphasized dimensions.
- Cache the analysis **per (user, job)** in a new `user_job_analysis` table, stamped with the
  CV's upload time and the job's `content_hash`. A cached result is served only while both
  stamps still match the current CV and job; otherwise it is reported **stale** and the SPA
  offers a recompute. Best-effort: with the LLM unconfigured or on error, the endpoint degrades
  (no analysis) and the deterministic bar is unaffected.
- Extend the **Profile match** UI block (`web/src/lib/components/JobMatch.svelte`): the fast,
  free deterministic bar stays on top; the AI analysis renders in an expandable section driven
  by the new endpoint.

## Capabilities

### New Capabilities
- `job-fit-analysis`: On-demand LLM analysis of a single (candidate, job) fit — the fixed
  three-stage prompt-chain (Extract&Match → Recruiter verdict → Adversarial audit), the
  five-dimension scored verdict + requirement-match table, its per-(user,job) cache with CV/job
  staleness invalidation, the authenticated GET/POST endpoints, and the SPA presentation
  extending the profile-match block.

### Modified Capabilities
<!-- None. The deterministic job-profile-match requirements are unchanged; this adds a
     sibling capability. The profile-match UI component is extended (implementation), but its
     deterministic-match behavior/requirements do not change. -->

## Impact

- **New code**: `internal/jobfit` (three-stage chain: per-stage prompt/parse/sanitize + the
  server-side weighted scoring, mirroring `internal/atscheck`), a handler
  (`internal/handler/job_fit.go`) with GET/POST on `/api/v1/jobs/:slug/fit`, a
  `user_job_analysis` migration + sqlc queries, TS contract via `cmd/gen-contracts`.
- **Reused**: `internal/llm` (`GenerateJSON`), `internal/jobmatch` (anchor), `internal/resume`
  (CV text + upload time), `internal/jobhash` (job `content_hash`), `internal/userprofile`.
- **Frontend**: `web/src/lib/components/JobMatch.svelte`, `web/src/lib/api.ts`, `web/src/lib/types.ts`.
- **Ops**: new migration must be applied manually before deploy (no versioned runner). No new
  config — reuses the existing `LLM_*` env; unconfigured LLM leaves the feature dormant.
