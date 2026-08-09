## Context

`ensureAnalysisForRun` (`internal/handler/assistant.go:771`) computes and caches the fit analysis
once, **before** `PostAssistantAutopilot` starts the agent's turn — purely so `cv_context` (the
run's first tool call) has something to read instead of erroring. Nothing runs after. The agent's
own judgment that an edit closed a requirement is never checked against anything.

The autopilot's own turn runs inside `streamSSE`'s detached SSE-writer goroutine
(`internal/handler/assistant.go:566`), on a `context.Background()`-derived `ctx` — the request's
`*fiber.Ctx` is released the instant `PostAssistantAutopilot` returns, well before the turn (or
anything that should run after it) executes. `internal/handler/match_analysis_stream.go`'s
`StreamMatchAnalysis` already solved exactly this constraint for the fit-analysis SSE endpoint:
everything that needs the fiber ctx (`companyInfo`, `candidateProfile`, `jobBlockers`, binding the
per-user LLM credential) is built **before** `SetBodyStreamWriter`, and only a plain `ctx` plus
already-built plain values are used inside the writer.

`tailor-autopilot`'s current spec (`### Requirement: The workspace shows the run report beside the
fit analysis`) states this as a hard rule: "the fit analysis it sits above MUST NOT be recomputed
by a run" / "the cached fit analysis shown beneath the report is the same one shown before the
run." `tailor-job-match`'s `### Requirement: The frozen fit analysis is labelled as a snapshot of
the base profile` states the same invariant from the labelling side. Both exist because the fit
analysis used to be the candidate's "should I bother tailoring this vacancy" signal, read on the
job listing page before any tailoring happened — a number that had to stay still while the
candidate mixed it mentally with the live `job_match` score. Cold-start autopilot
(`tailor-coldstart-autopilot`, superpowers doc `docs/superpowers/specs/2026-08-09-tailor-coldstart-autopilot-design.md`)
already removed that product moment: opening Tailor now starts editing immediately, so there is no
"before" reading left to protect.

## Goals / Non-Goals

**Goals:**
- Give the autopilot run a way to independently verify its own edits before it reports, using the
  free, deterministic `job_match` tool (`internal/handler/assistant_cv_tools.go:210`) rather than
  its own belief about what it changed.
- Guarantee the cached `(user, job)` fit analysis reflects the outcome of the latest autopilot run,
  unconditionally — including a run that made zero edits.
- Repeal the "frozen base-profile snapshot" invariant in `tailor-autopilot` and `tailor-job-match`
  now that its underlying product assumption no longer holds.

**Non-Goals:**
- Restructuring `internal/matchanalysis`'s three-stage chain itself (Extract & Match / Recruiter
  verdict / Adversarial audit). The cost concern that raised this is already solved structurally —
  see Decision 4.
- Any DB schema change. Still exactly one `user_job_analysis` row per `(user, job)`; a second CV
  tailored later for the same vacancy overwrites it (accepted, not solved here).
- Live-updating `cv_context` mid-run. It keeps serving whatever the last full analysis found; the
  fresh `job_match` per-requirement status is what tracks progress within a run.

## Decisions

### 1. Two independent mechanisms, not one

**Self-check loop (prompt/agent layer, soft-bounded):** the Tailor preset's `UNATTENDED RUNS`
prompt section instructs the agent to call `job_match` before it calls `tailor_report`, and to keep
editing (soft cap 2-3 rounds, agent's own judgment to stop) while a closeable `missing_have` or
`missing_gap` remains. No numeric score gate: `job_match`'s weighted overall mixes Title Match (20)
and Seniority Fit (10) — categories a text edit cannot move — so a fixed bar like "stop at 75"
either stalls on a number editing can never reach, or is meaningless when title/seniority were
already fine.

**Guaranteed final refresh (server layer, unconditional):** a new step,
`prepareAutopilotRun`/its returned closure, mirrors `ensureAnalysisForRun` at the other end of the
run — after the agent's turn completes, it recomputes the full three-stage chain and overwrites
the cached row, **every time**, regardless of what the agent did or didn't edit. This cannot be
left to the agent's own tool discretion: "always, even when nothing changed" is a guarantee, not a
judgment call.

**Alternative considered:** a single mechanism where the agent itself decides whether to trigger
the final recompute (e.g. by calling a new `refresh_fit_analysis` tool). Rejected — an agent that
forgets, or judges no edits were needed, would silently leave a stale cache, and "the cache is
current" needs to be an invariant the server owns, not something contingent on model behavior.

### 2. Where the guaranteed refresh actually runs

`PostAssistantAutopilot` stops delegating to the generic `streamTurn` helper and instead calls
`streamSSE` directly with its own `start` closure — the same shape `streamTurn`/`streamContinue`
already use, just with one extra line:

```go
refreshAnalysis := h.prepareAutopilotAnalysis(c, sess)
h.layDownRunPlan(c.Context(), sess)
return h.streamSSE(c, sess, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
    err := runner.Run(ctx, sess, reg, system, autopilotBrief, assistant.TurnConfig{MaxSteps: autopilotMaxSteps}, emit)
    refreshAnalysis(ctx)
    return err
})
```

`prepareAutopilotAnalysis` (replacing today's `ensureAnalysisForRun`) resolves the job and
delegates to a new `matchHandlers.prepareAutopilotRun(c, userID, job) func(context.Context)`,
which:
1. Runs today's `ensureCachedAnalysis` (fill-if-empty) synchronously, while `c` is still valid —
   unchanged behavior.
2. Builds the fit-chain `Input` and a bound `*matchanalysis.Analyzer` once, using `c` — also while
   still valid.
3. Returns a closure over that `Input`/`Analyzer` that, given a plain `ctx`, unconditionally runs
   the chain and overwrites the cache (`cacheAnalysis`, which already takes a plain `ctx` for
   exactly this reason).

This means the refresh executes inside the **same** SSE-writer goroutine as the turn itself,
sequentially after `runner.Run` returns — not a second detached goroutine. The turn's own SSE
events have already reached the client by that point (`emit` already fired the terminal event), so
the extra work only delays how soon the underlying connection closes, not the perceived
turn-completion latency for the reader.

**Alternative considered:** spawn a second, independent `go func()` for the refresh so the HTTP
stream can close immediately after the turn. Rejected as unnecessary complexity — the refresh is a
handful of LLM calls that the connection lifetime doesn't gate (SSE responses in this codebase
already run detached, long-lived writes; see `match_analysis_stream.go`'s `context.Background()`
comment), and a bare `go func()` here would need its own error handling/lifetime story for no
measured benefit.

### 3. `buildAnalysisInput` extraction is a prerequisite, not incidental

`runAnalysis` today builds the `matchanalysis.Input` and calls `Analyze` in one step, both against
a live `c *fiber.Ctx`. `prepareAutopilotRun` needs the `Input` built while `c` is valid but the
actual `Analyze` call to happen later against a plain `ctx`. Splitting `buildAnalysisInput` out of
`runAnalysis` is the minimal change that allows this without duplicating the twelve-field literal
or changing `runAnalysis`'s own callers (`PostMatchAnalysis`, `ensureCachedAnalysis`).

### 4. Chain restructuring stays out of scope

The three-stage chain now runs exactly **twice** per cold start (once before the run via
`ensureCachedAnalysis`, once after via the guaranteed refresh) instead of once — but the self-check
loop's per-iteration cost is zero LLM calls, since it runs entirely on the free `job_match` tool.
Restructuring the chain itself (merging stages, conditionally skipping the adversarial audit) would
trade prompt/quality risk for a cost reduction that isn't proven necessary yet. Revisit only if the
2x-per-run cost is measured to matter after shipping.

## Risks / Trade-offs

- **[Risk] LLM spend roughly doubles per cold-start tailoring session** (one extra full 3-stage
  chain run). → **Mitigation:** unmetered (no AI-credit debit), same as today's pre-run compute;
  real LLM token cost is a known, accepted trade-off per Decision 4, not something this change can
  silently avoid.
- **[Risk] A candidate reopening a vacancy's fit analysis outside Tailor (job listing drawer) will
  see whatever a past tailoring session produced, not their untouched base-profile score** — a
  behavior change, not a bug, but one a support inquiry could surface. → **Mitigation:** this is
  the explicit intent (see proposal's "BREAKING (behavioral)" note); no code path silently reverts
  it.
- **[Risk] The refresh's LLM call fails or times out.** → **Mitigation:** best-effort, matching
  `internal/matchanalysis/AGENTS.md`'s existing "best-effort throughout" rule — on failure the
  previous cached analysis is left as-is; the autopilot turn's own success/failure is unaffected
  (the refresh runs after `runner.Run` has already returned).

## Migration Plan

No data migration. Deploy order is not load-bearing between the prompt change (Task 3) and the
server change (Tasks 1-2) — either can ship first without breaking the other, since `job_match` is
already registered as a tool today regardless of whether the prompt mentions it. Rollback is a
plain revert; no cache row format changes.

## Open Questions

None outstanding — resolved during design discussion:
- Post-run refresh ordering/goroutine question → resolved by Decision 2 (same goroutine,
  sequential after `runner.Run`).
- Whether `internal/llmkey`'s background-entrypoint scope test constrains this path → does not
  apply; that test only restricts `cmd/*` binaries from importing `internal/llmkey` directly. This
  change lives in `internal/handler` (the `cmd/server` binary) and resolves the user's own
  credential exactly as `ensureCachedAnalysis` does today — "unmetered" here means "never calls
  `credits.Debit`," which the implementation simply never does.
