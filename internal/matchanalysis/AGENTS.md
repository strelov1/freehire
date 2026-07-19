# AI fit analysis conventions

## Scope
On-demand, cached, three-stage LLM prompt-chain for job-fit analysis per (user, job). Backend `internal/jobfit`; frontend analysis page in `web/src/routes/jobs/[slug]/fit/`.

## Always true
- **Fixed prompt-chain, NOT an autonomous agent.** Deterministic, typed, cacheable. Runs over the shared `internal/llm` client — provider-agnostic, no vendor baked in.
- **Stage 1 Extract & Match:** extract posting requirements, classify each against CV as `covered`/`synonym-only`/`missing-have`/`missing-gap`. Never fabricate a skill.
- **Stage 2 Recruiter verdict:** six scored dimensions (title alignment, experience relevance, seniority fit, skills coverage, company context, location & work-mode fit). Model only scores dimensions — the server computes `overall_score` and `verdict`.
- **Stage 3 Adversarial audit:** skeptic pass that refines Stage 2. Stage 3 merges onto Stage 2 (unmarshalled over a copy of sanitized Stage-2 verdict); a parse failure degrades to the un-audited Stage-2 verdict.
- **`overall_score` is server-owned** (named-weight average: Title 20 / Experience 25 / Seniority 15 / Skills 15 / Company 10 / Location 15). The model never computes this — ensures consistency and testability.
- **All model output is sanitized** to controlled vocabulary before persisting or serving (scores clamped, statuses coerced, text bounded). Same "never persist an out-of-vocabulary value" invariant as enrichment. This is also the prompt-injection guard for untrusted `description`/`company_info`.
- **Cache is triple-stamped:** CV upload time, job `content_hash`, and model. GET reports the row stale when any differs from live values. A `content_hash` absent on both sides counts as unchanged (non-board jobs carry none). Model stamp invalidates on `LLM_MODEL` upgrade.
- **GET never calls the LLM** — serves cache or null. POST runs the chain and upserts.
- **Best-effort throughout:** an unconfigured/failing LLM leaves the deterministic `jobmatch` bar untouched and returns no analysis.
- **All endpoints are `RequireAuthOrKey`.**
- **Location degrades gracefully:** a profile with no `location_preferences` is scored on job geography alone, never an error.

## How it works

`internal/jobfit` is complemented by the deterministic `internal/jobmatch` bar (skills-only, instant, free). The LLM analysis is opt-in and reads the whole vacancy + `company_info` + the caller's stored CV.

**The chain:** `jobfit.go` defines the `Analysis` wire shape and `AnalyzeStream(ctx, in, emit)` — the one chain implementation. `analyzer.go` is a thin collector over it. `Analyze` is the sync entry point that collects stream events into the final `Analysis`.

**SSE streaming:** `GET /jobs/:slug/fit/stream` opens an SSE endpoint (`SetBodyStreamWriter` with `X-Accel-Buffering: no`). Events: `stage_start`/`stage_done` (3-step stepper), `thinking` (reasoning-token deltas via `llm.GenerateJSONStream`'s `WithStreamingReasoningFunc` — empty on non-reasoning models; raw JSON tokens are never surfaced), and each section as it resolves (`requirements`→S1, `dimensions`→interim S2, `final`→audited). The final result is cached exactly as the sync path on completion.

**Frontend:** a dedicated full-width analysis page SSRs a fresh cached analysis via `+page.server.ts` for instant paint; otherwise opens an `EventSource` with a stepper, thinking panel, and progressive sections. The pure SSE reducer `reduceFitEvent` lives in `web/src/lib/jobFit.ts` (unit-tested). The Profile-match sidebar block (`JobFitAnalysis.svelte`) is a compact summary linking to the page — it never computes inline.

**Structured resume context:** the `resumeextract` wire shape is fed into the fit chain as pre-normalized Stage-1 context (`jobfit.Input.StructuredResume`) — additive, never a replacement. A missing/failed extraction degrades to text-only analysis.

**Code generation:** wire shape generated to TS via `cmd/gen-contracts`.

## Limitations
- A live company web-research stage (Stage 2a) is where a real tool-using agent would fit later; company context is `company_info`-only for now.
- Migration numbering: parallel branches produced several `0009_*` files (job-analysis, daily-stats, profile-location→renamed `0010_`); harmless because Postgres initdb runs by filename, but a versioned runner is the real fix.
- `user_job_analysis` migration (`0009`) must be applied to prod manually before deploy.
