# AI fit analysis conventions

## Scope
On-demand, cached, three-stage LLM prompt-chain for job-fit analysis per (user, job). Backend `internal/candidate/matchanalysis`; frontend embedded in the Tailor workspace (`web/src/routes/tailor/[slug]/`), with `web/src/routes/match/[slug]/` kept only as a 308 redirect to it.

## Always true
- **Fixed prompt-chain, NOT an autonomous agent.** Deterministic, typed, cacheable. Runs over the shared `internal/platform/llm` client — provider-agnostic, no vendor baked in.
- **Stage 1 Extract & Match:** extract posting requirements, classify each against CV as `covered`/`synonym-only`/`missing-have`/`missing-gap`, and grade the cited evidence of the two positive statuses as `evidence_strength` `metric`/`scope`/`responsibility`/`keyword` (coerced to `keyword` when unknown; empty for `missing-*`). Never fabricate a skill.
- **Stage 2 Recruiter verdict:** six scored dimensions (title alignment, experience relevance, seniority fit, skills coverage, company context, location & work-mode fit). Model only scores dimensions — the server computes `overall_score` and `verdict`.
- **Stage 3 Adversarial audit:** skeptic pass that refines Stage 2. Stage 3 merges onto Stage 2 (unmarshalled over a copy of sanitized Stage-2 verdict); a parse failure degrades to the un-audited Stage-2 verdict.
- **`overall_score` is server-owned** (named-weight average: Title 20 / Experience 25 / Seniority 15 / Skills 15 / Company 10 / Location 15). The model never computes this — ensures consistency and testability.
- **All model output is sanitized** to controlled vocabulary before persisting or serving (scores clamped, statuses coerced, text bounded). Same "never persist an out-of-vocabulary value" invariant as enrichment. This is also the prompt-injection guard for untrusted `description`/`company_info`. Sanitize ceilings are process-wide (`matchanalysis.SetBounds`); `cmd/server` loads them from `MATCH_ANALYSIS_*` (defaults in `DefaultBounds` / `.env.example`).
- **The cache, the staleness stamp, the credit rule and the coalescing are NOT here** — they live in
  [`internal/candidate/fitanalysis`](../fitanalysis/AGENTS.md), because the autopilot's two invisible
  halves reach them with no HTTP request at all. This package is the chain, the `Analysis` type and
  its sanitize ceilings.
- **Cache is quadruple-stamped:** CV upload time, job `content_hash`, model, and the caller's profile language. GET reports the row stale when any differs from live values. A `content_hash` absent on both sides counts as unchanged (non-board jobs carry none). Model stamp invalidates on `LLM_MODEL` upgrade. Language stamp invalidates when the caller switches their profile language (freehire#1837) — every free-text field (dimension comments, strengths, gaps, recommendation, hidden-signal insights) is written in it via `Input.Language`, since it is the candidate's own reading of their fit, not text that goes onto a CV.
- **GET never calls the LLM** — serves cache or null. POST runs the chain and upserts.
- **Best-effort throughout:** an unconfigured/failing LLM leaves the deterministic `jobmatch` bar untouched and returns no analysis.
- **All endpoints are `RequireAuthOrKey`.**
- **Location degrades gracefully:** a profile with no `location_preferences` is scored on job geography alone, never an error.

## How it works

`internal/candidate/matchanalysis` is complemented by the deterministic `internal/candidate/jobmatch` bar (skills-only, instant, free). The LLM analysis is opt-in and reads the whole vacancy + `company_info` + the caller's de-identified structured résumé.

**The chain:** `analyzer.go` holds the one chain implementation, `AnalyzeStream(ctx, in, emit)` — a method on `*Analyzer`. `Analyze` is the sync entry point, a one-line wrapper that runs it with a no-op emit. `matchanalysis.go` holds the `Analysis` wire shape, the sanitize pass, and the weighted scoring.

**SSE streaming:** `GET /jobs/:slug/match-analysis/stream` opens an SSE endpoint (`SetBodyStreamWriter` with `X-Accel-Buffering: no`). Events: `stage_start`/`stage_done` (3-step stepper), `thinking` (reasoning-token deltas via `llm.GenerateJSONStream`'s `WithStreamingReasoningFunc` — empty on non-reasoning models; raw JSON tokens are never surfaced), and each section as it resolves (`requirements`→S1, `dimensions`→interim S2, `final`→audited). The final result is cached exactly as the sync path on completion.

**Frontend:** the standalone analysis page was removed; the Tailor workspace is the sole surface. `MatchAnalysisFull.svelte`, embedded in the workspace, fetches the cached analysis and otherwise opens an `EventSource` with a stepper, thinking panel, and progressive sections. `web/src/routes/match/[slug]/+page.server.ts` remains only as a 308 redirect to `/tailor/[slug]`, so old links keep working. The pure SSE reducer `reduceMatchEvent` lives in `web/src/lib/matchAnalysis.ts` (unit-tested). The Profile-match sidebar block (`MatchSummary.svelte`) is a compact summary linking to the workspace — it never computes inline.

**Structured resume context:** `resumeextract.Professional` — the contact-free projection, as a TYPE, not a convention about a JSON string — is the SOLE candidate context of the fit chain (`matchanalysis.Input.StructuredResume`). The raw CV text is never sent to the model, and this package strips nothing: a field the projection does not name cannot arrive here. A candidate with no banked experience means NO analysis is produced (the endpoint degrades to `has_cv` with a null analysis); there is deliberately no text-only fallback and no fallback to the structure's own copy of the work history.

**Code generation:** wire shape generated to TS via `cmd/gen-contracts`.

## Limitations
- A live company web-research stage (Stage 2a) is where a real tool-using agent would fit later; company context is `company_info`-only for now.
- Migration numbering: parallel branches produced several `0009_*` files (job-analysis, daily-stats, profile-location→renamed `0010_`); harmless because Postgres initdb runs by filename, but a versioned runner is the real fix.
- `user_job_analysis` migration (`0009`) must be applied to prod manually before deploy.
