# Fit analysis: verify after autopilot, not just before — design

> **Superseded:** the change shipped as the OpenSpec change
> `openspec/changes/fit-analysis-post-autopilot-verify/` (proposal/design/specs/tasks) — that
> `design.md` is current, including a context-cancellation fix found during implementation review
> that this doc predates. Kept here as the original architectural rationale.

Builds on `2026-08-09-tailor-coldstart-autopilot-design.md` (fit analysis computed
in the background around the cold-start autopilot run) and repeals part of the
`2026-07-31-tailor-job-match-tab` change (the "cached fit analysis is a frozen
snapshot of the base profile, never the tailored document" rule).

## Problem

Today `ensureAnalysisForRun` (`internal/handler/match_analysis.go:256`, called
from `PostAssistantAutopilot`) computes the fit analysis **once, before** the
tailoring agent's turn starts, purely as a precondition — `cv_context` 409s
without it. Nothing re-checks it, or anything else, **after** the agent's edits
land. The agent finishes a run trusting its own judgment that it closed a gap;
nothing independently confirms the edit actually reads that way to the same
deterministic logic that will later score the CV for real.

Separately, the Job Match tab design (`2026-07-31-tailor-job-match-tab`)
deliberately keeps the cached fit analysis frozen as a "snapshot of the base
profile," specifically so it is never mixed with `job_match`'s live,
tailored-document score — "no tab SHALL mix measurements taken against
different baselines." That invariant assumed a moment in the product where a
candidate decides *whether to bother tailoring* by reading the base-profile
number on the job listing drawer, separate from *how tailoring went*. Cold-start
autopilot already collapsed that decision — opening a vacancy in Tailor
immediately starts editing — so the frozen "before" number no longer serves a
use case anyone has. Confirmed in discussion: drop it.

## Decision

Two independent mechanisms, one per layer — do not conflate them.

### 1. Self-check loop — agent/prompt layer, soft-bounded

The tailoring agent's own tool-calling loop (Tailor/autopilot preset,
`internal/assistant`) is instructed, in its prompt, to call the existing
`job_match` tool (`internal/handler/assistant_cv_tools.go:210` — no LLM call,
free, recomputes fresh off the CV's own rendered text) after a batch of edits,
and to keep editing for as long as it judges a closeable `missing_have` or
`missing_gap` remains. Soft cap: **2–3 iterations**, and the agent may stop
earlier on its own judgment.

No numeric score gate. `job_match`'s weighted overall mixes Title Match (20)
and Seniority Fit (10) — categories a text edit cannot move — so a fixed bar
like "stop at 75" either stalls on a number editing can never reach, or is
meaningless when title/seniority were already fine. The agent already holds
`cv_context`'s classified requirement list (`missing_have`/`missing_gap`); what
it lacks is an independent check that an edit *actually reads* as closing one.
`job_match` is that check — it is the same code that will score the CV for
real, not the agent's opinion of its own work.

This ships as a prompt change (`internal/assistant/prompt.go` or wherever the
Tailor/autopilot preset is defined), not new server code — both tools already
exist and are already available to this preset.

### 2. Guaranteed final refresh — server layer, unconditional

A new deterministic step, `refreshAnalysisAfterRun`, mirrors
`ensureAnalysisForRun` at the other end of the same autopilot turn: once the
agent's run completes, it recomputes the full three-stage chain
(`matchanalysis.Analyzer.Analyze`) and overwrites the cached `(user, job)` row —
**every time, even when the agent made zero edits.**

This cannot be left to the agent's own tool discretion (mechanism 1). "Always,
even when nothing changed" is a guarantee, not a judgment call, so it is
unconditional server code called from `PostAssistantAutopilot` after the turn
finishes, not something the model chooses to invoke. Like `ensureCachedAnalysis`
today, it stays **unmetered** (no AI-credit debit) — same background-compute
convention, same "fails open" LLM-spend attribution.

On an LLM failure/timeout during this refresh: keep the previous cached
analysis rather than blocking or clearing anything — the existing "best-effort
throughout" rule in `internal/matchanalysis/AGENTS.md` already covers this
shape of degradation.

## Consequence: the "snapshot of the base profile" rule is repealed

The cached `(user, job)` fit-analysis row is no longer a frozen pre-tailoring
snapshot. `openspec/specs/tailor-workspace/spec.md`'s Job Match tab requirement
— "the cached fit analysis beneath it labelled as a snapshot of the base
profile" — becomes wrong the moment `refreshAnalysisAfterRun` ships, and needs
its own MODIFIED delta: the tab shows one number, kept current by the latest
autopilot run, not two numbers on two baselines.

The job listing drawer (`JobDrawer.svelte`, `autoRun=true`) is untouched
code-wise — it still reads/computes the same cache row — but what that row
*means* changes: a candidate who has tailored a CV for a vacancy will see that
tailoring's result on the job listing page too, not a separate "before"
number. Confirmed acceptable — there is deliberately no longer a "should I
bother tailoring" moment distinct from "how did tailoring go."

No DB schema change: still one row per `(user, job)`. A second CV tailored
later for the same vacancy overwrites the same row — an accepted trade-off,
not solved by this design.

## Out of scope, deliberately

- **Restructuring the three-stage chain itself** (Extract & Match / Recruiter
  verdict / Adversarial audit). The cost concern that raised this is already
  addressed structurally: the interim touch-up loop runs entirely on the free
  `job_match`, so a cold-start now costs exactly **2 full chain runs** (before,
  after) regardless of how many touch-up iterations the agent takes inside the
  run — versus 1 today. Revisit only if 2x turns out to matter after shipping.
- **Live-updating `cv_context` mid-run.** It keeps serving whatever the last
  full analysis found; the agent's own reasoning plus the fresh `job_match`
  per-requirement status is what tracks progress within a run.

## Open questions for the implementation plan

1. Exact call site for `refreshAnalysisAfterRun` inside `PostAssistantAutopilot`
   — synchronous before responding, or background goroutine like the
   cold-start's parallel fit-analysis compute (per the cold-start-autopilot
   design)? Affects perceived turn-completion latency.
2. Prompt wording for the self-check loop (`internal/assistant/prompt.go` or
   the Tailor/autopilot preset definition) — needs to reuse `cv_context`'s
   `missing_have`/`missing_gap` vocabulary so the two tools read as one
   language, not two competing signals.
3. Confirm `refreshAnalysisAfterRun` is unmetered the same way
   `ensureCachedAnalysis` is (`internal/handler/match_analysis.go:253-255`) and
   is covered by the existing test that background entrypoints never resolve a
   user's LLM credit key (`internal/llmkey` convention, per root AGENTS.md).
4. UI copy: new wording for the Job Match tab's fit-analysis panel now that
   it is not a "snapshot" (`MatchAnalysisFull.svelte` / `ArtifactPanel.svelte`).
   Cosmetic, but the current label is actively wrong once this ships.
