## Context

The job page's **Profile match** (`internal/jobmatch` + `web/.../JobMatch.svelte`) is a pure,
deterministic skills set-operation over canonical skilltag slugs. It is fast and free but blind to
the vacancy text, company context, and the candidate's actual experience — so it can't judge
job-title alignment or experience relevance, the two signals an ATS keyword-screen and a recruiter
weigh most.

The codebase already has the exact building blocks:
- `internal/llm` — `Client.GenerateJSON(ctx, system, user) (string, error)`; nil client ⇒ feature dormant.
- `internal/atscheck.Analyzer` + `handler.PostATSReport` — the proven "on-demand LLM review, sanitize
  untrusted output, cache per user, best-effort degrade" pattern. This change mirrors it, but keyed
  per (user, **job**) instead of per user.
- `internal/resume` — stored CV text (`Text`) and the CV upload time (`Status().UploadedAt`).
- `internal/jobmatch` — the deterministic match reused as an anchor.
- `internal/jobhash` — the job `content_hash` already computed at ingest, reused for staleness.
- `cmd/gen-contracts` — generates the TS wire type from the Go struct.

## Goals / Non-Goals

**Goals:**
- A high-quality analysis per explicit user action: a fixed three-stage prompt-chain
  (Extract&Match → Recruiter verdict → Adversarial audit) producing a five-dimension scored
  verdict centered on title alignment and experience relevance, plus the ATS-style
  requirement-match table.
- Honest caching: never present a stale analysis (CV changed or job re-ingested) as current.
- Zero impact on the deterministic bar and on envs without an LLM configured.

**Non-Goals:**
- No **autonomous** LLM agent (see Decision 7) — the chain is a fixed, deterministic sequence.
- No live web research of the company (company_info only; the live-research stage is a marked
  future seam — see Decision 8).
- No background queue / precompute worker (an on-demand button + cache is enough for now; note the
  seam — a `user_job_analysis` outbox could be added later mirroring `enrichment_outbox`).
- No cover-letter / CV-tailoring generation (the reference project does this; out of scope).
- No new LLM config or provider coupling — reuse `LLM_*`.
- No change to deterministic `job-profile-match` requirements.

## Decisions

**1. New `internal/jobfit` package, mirroring `internal/atscheck`.** Owns the three stages'
prompt builds, the per-stage untrusted-output sanitize, and the deterministic weighted scoring.
Each stage is a plain function taking typed input and returning a typed struct, so the whole
`Analyzer` is unit-testable with a fake LLM client that returns canned per-stage JSON, matching
`atscheck.Analyzer`. *Alternative:* extend `jobmatch` — rejected: `jobmatch` is deliberately
pure/deterministic/no-LLM; mixing an LLM dependency in would break that boundary.

**1a. Three fixed stages, each a typed `GenerateJSON` call.**
- **Stage 1 — Extract & Match** → `RequirementMatch{ Requirements: []Requirement{Text, Priority
  (required|preferred), Status (covered|synonym-only|missing-have|missing-gap), Evidence} }`.
  Input: job title+description + CV text + deterministic anchor. This is the ATS lens.
- **Stage 2 — Recruiter verdict** → the five `Dimension` scores + `strengths`/`gaps`/
  `recommendation`. Input: Stage 1's `RequirementMatch` + company_info + CV + anchor.
- **Stage 3 — Adversarial audit** → a corrected copy of the Stage 2 verdict (adjusted scores,
  pruned unsupported strengths, surfaced missed gaps). Input: Stage 2 output + the same evidence.
The server then computes `overall_score`/`verdict` from Stage 3's dimensions (Decision 2). A stage
that fails to parse degrades: Stage 3 failing falls back to the Stage 2 verdict; Stages 1–2 failing
degrade to no-analysis (best-effort, Decision below).

**2. Server owns `overall_score` and `verdict`; the model only scores dimensions.** We compute the
weighted average server-side (Title 25% / Experience 25% / Seniority 15% / Skills 20% / Company 15%
— weights are a named, tunable constant) and derive the verdict label from thresholds, rather than
trusting the model's own overall. Keeps the headline number deterministic and consistent with the
dimensions even when the model is internally inconsistent. *Alternative:* trust the model's overall —
rejected: models drift between per-dimension scores and their own summary.

**3. Deterministic match fed into the prompt as an anchor.** `jobmatch.Compute(job.Skills,
profile.Skills)` runs first; its exact/adjacent/missing slugs + coverage percent go into the user
prompt. The model explains and augments (hidden/transferable experience) instead of recomputing
skills — reduces hallucination and keeps Skills coverage consistent with the free bar.

**4. Cache keyed `(user_id, job_id)` with triple staleness stamps.** New table
`user_job_analysis (user_id BIGINT, job_id BIGINT, analysis JSONB, model TEXT, cv_uploaded_at
TIMESTAMPTZ, job_content_hash TEXT, created_at TIMESTAMPTZ DEFAULT now(), PRIMARY KEY (user_id,
job_id))`, FKs to `users`/`jobs` ON DELETE CASCADE. `POST` computes + upserts; `GET` reads and
compares three stamps — CV upload time, job `content_hash`, and **model** — against the live values,
returning a `stale` boolean. The model stamp invalidates the cache on an `LLM_MODEL` upgrade so the
improved model re-analyzes (the analogue of the enrichment-version and semantic-embedder guards). A
`content_hash` absent on both sides counts as unchanged: non-board jobs (telegram/habr) carry none
and are never re-crawled, so a NULL must not force an endless recompute; a hash on one side only is a
change. *Alternative:* a single JSONB column on `user_jobs` — rejected: `user_jobs` is the interaction
row (view/apply/save/track), a different concern; a dedicated table keeps the analysis lifecycle
independent and lets it exist without an interaction.

**4a. Stage 3 audit merges onto Stage 2, not replaces.** The audit response is unmarshalled onto a
copy of the already-sanitized Stage 2 verdict, so a budget model that echoes only the fields it
changed refines the verdict (present keys override) instead of hollowing it out (omitted dimensions
keep their Stage 2 scores). Without this, a partial `{...}` audit would zero the un-returned
dimensions and collapse a strong overall — the audit stage would corrupt rather than sharpen.

**5. GET reads, POST computes.** `GET` never calls the LLM: it returns the cached analysis (with
`stale`) or a null analysis when none exists — cheap, safe to call on expand. `POST` runs the LLM,
upserts, and returns fresh — bound to the explicit button. Mirrors `GetATSReport`/`PostATSReport`.

**6. Wire shape.** `jobfit.Analysis` is the single Go struct → TS via `cmd/gen-contracts`; the
handler wraps it as `{ "data": { "has_cv": bool, "stale": bool, "analysis": Analysis|null } }`,
consistent with `atsResponse`. `Analysis` carries the five dimensions, the Stage 1
`requirement_match` table, `overall_score`, `verdict`, `strengths`, `gaps`, `recommendation`.

**7. Fixed prompt-chain, NOT an autonomous langchain agent.** The three stages run as a fixed
server-orchestrated sequence of typed `GenerateJSON` calls over the existing `llm.Client`
(langchaingo under the hood). We deliberately do **not** use langchaingo's `agents` (ReAct)
package. *Why:* an autonomous agent decides its own steps/tool-calls, which makes cost, latency,
and output shape non-deterministic — the opposite of what a cached, unit-tested, bounded product
endpoint needs. Our step count and order are known ahead of time, each stage has a typed JSON
contract, and every input (CV, job, company_info, deterministic anchor) is already in memory before
the first call, so there is no tool for an agent to decide to invoke. The reference project's
"drafter→reviewer" is likewise two fixed roles, not a tool-loop — it maps to our fixed Stage 2 →
Stage 3, not to a ReAct agent. *Alternative:* a tool-using agent — rejected now; see Decision 8 for
the one future case that would justify it.

**8. Company context = stored `company_info` only; live web research is a future seam.** Stage 2
grounds the "company angle" from the already-stored `company_info` JSONB (bounded, injected into
the prompt) — free and instant, no web tool on the backend. The reference's live company WebSearch
(its Step 3 reviewer) is intentionally deferred: it is expensive and slow per job-view and needs a
web-search tool the backend does not have. *Seam:* if a live-research "company deep-dive" is added
later (e.g. a premium tier), THAT stage is the one place a real tool-using langchaingo agent is the
right abstraction (loop: search → read → decide-if-enough); it would slot in as an optional Stage 2a
without disturbing the fixed chain.

## Risks / Trade-offs

- **3× LLM cost + latency per analysis** (three sequential calls) → mitigated by cache + explicit
  action + no auto-fetch; a stale result recomputes only on user request. A per-stage timeout
  bounds a stalled gateway; Stage 3 failing degrades to the Stage 2 verdict rather than erroring.
- **Serial-stage latency** (three round-trips before a result) → acceptable for an explicit,
  spinner-backed action; the SPA shows progress and the result is then cached.
- **Prompt injection via job description / company_info** (untrusted ingested text in the prompt) →
  mitigated by clamping/coercing all output to the controlled contract (scores, verdict enum,
  bounded text) and never executing model output; the model can't escape the JSON contract into a
  harmful action.
- **CV text leakage into logs** → follow `atscheck`: log failures without the CV/job text.
- **Weight calibration is a guess** → weights and the must-fit thresholds are named constants,
  unit-tested, and tunable without touching call sites.
- **Migration has no versioned runner** (repo gotcha) → the `user_job_analysis` migration must be
  applied to prod manually before deploy; documented in tasks.

## Migration Plan

1. Add `migrations/00NN_user_job_analysis.sql`; regenerate sqlc.
2. Ship backend (endpoint dormant without `LLM_*`), apply the migration to prod manually, then the
   frontend. Rollback: drop the table + revert; nothing else depends on it.

## Open Questions

- Exact dimension weights and verdict thresholds — start from the reference (Technical 30 /
  Experience 25 / Behavioral 15 / Career 30) adapted to our five dimensions; calibrate later.
