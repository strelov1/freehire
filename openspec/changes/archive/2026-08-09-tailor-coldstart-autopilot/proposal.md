## Why

Live QA of PR #1658 (match-tailor-merge) surfaced two problems that make Tailor's cold start worse
than it should be. First, `experienceFromBank()` seeds a brand-new tailored CV with every
publishable atom under every employment, unbounded — as a candidate accrues more banked
achievements over time, every subsequently-seeded CV gets longer and less curated, the opposite of
what a résumé needs. Second, the tool that already fixes this — the JD-aware "Tailor it for me"
autopilot run, which walks the vacancy's requirements against the bank and picks relevant evidence
— is a manual, opt-in action reachable only after the cold-start bootstrap (still gated behind a
cached fit analysis by #1658's "analyzing" flow) has already completed. The fix that exists isn't
the default.

## What Changes

- **BREAKING**: The tailoring bootstrap (`internal/cv.Store.Tailor`) no longer requires a cached
  fit analysis to exist. The `409 "run the fit analysis first"` gate is removed entirely.
- On a first-time bootstrap for a vacancy, the bootstrap response flags a cold start; the workspace
  reacts by immediately starting the JD-aware autopilot curation run itself (the same call its own
  "tailor it for me" action makes) instead of offering a two-action menu — the workspace is
  interactive right away, no waiting on either the analysis or the run before it opens.
- The AI fit analysis (`internal/matchanalysis`) is computed as an inline precondition of that same
  autopilot run when no cached analysis exists yet, and populates the Job Match tab whenever it
  lands — the same "surfaces the delta without being asked" pattern already used for the
  ATS-readability delta, applied to the fit analysis too.
- Frontend: the CV preview shows the autopilot run happening live — bullets and sections filling
  in as the agent works — not a single before/after swap at turn end.
- **BREAKING**: PR #1658's `status: 'analyzing'` branch, `retryBootstrapAfterAnalysis`, and the
  inline `MatchAnalysisFull` render in `web/src/routes/tailor/[slug]/+page.svelte` are removed —
  unreachable once the backend never 409s for "no analysis".
- Failure/degradation: if the autopilot run fails outright (no LLM configured, empty bank, agent
  error), cold start falls back to the current unfiltered `Seed()` bootstrap rather than leaving
  the candidate on a dead screen.
- Out of scope, deferred: capping `experienceFromBank()`'s bullet count for the *base*
  (vacancy-less) CV — it still has no JD to curate against, so it keeps its current unfiltered seed.
  The colored-underline overlay (a separate, already-speced sub-project) is untouched.
- The other 409 in this path — "add a résumé first" when no base CV can be seeded — is untouched;
  it is a structurally separate check from the fit-analysis gate being removed.

## Capabilities

### New Capabilities

(none — this reshapes cold-start behavior inside existing capabilities rather than introducing a
new one)

### Modified Capabilities

- `cv-tailoring`: the "Tailoring requires an existing fit analysis" requirement (the 409 gate) is
  removed. The bootstrap's contract changes from "returns the cached analysis, 409s without one" to
  "always succeeds once a base CV can be seeded, and flags whether this call just started a cold
  start." The "surfaced only after analysis" clause of the beta-gating requirement is also revisited,
  since there is no longer an analysis-gated entry point to hide behind.
- `job-fit-analysis`: the "Fit analysis surfaces in the tailoring workspace" requirement (added by
  #1658) described the workspace opening the fit-analysis stream inline in reaction to the
  bootstrap's 409, then retrying the bootstrap. That reactive, blocking flow is replaced by the
  analysis being computed inline inside the automatically-triggered autopilot run itself.
- `tailor-workspace`: the requirement that a freshly bootstrapped session "MUST NOT start talking
  on its own" and instead offers two actions (run unattended / walk the gaps in conversation) no
  longer holds for a brand-new cold start — the workspace starts the autopilot run itself
  automatically instead of showing that menu.
- `tailor-autopilot`: gains the analysis-precondition behavior (compute inline when missing, reuse
  when cached) and its own spend-attribution tag for a cold-start-triggered run.

## Impact

- Backend: `internal/handler/cv_tailor.go` (`TailorCV`'s response gains `cold_start_running`),
  `internal/handler/assistant.go` (`PostAssistantAutopilot` gains the inline analysis precondition
  and a cold-start spend-attribution tag), `internal/matchanalysis` (reused compute path, called
  inline instead of gating the old bootstrap).
- Frontend: `web/src/routes/tailor/[slug]/+page.svelte` (bootstrap response handling, removal of
  the `'analyzing'` branch and `retryBootstrapAfterAnalysis`, auto-triggering the autopilot call),
  `web/src/lib/tailor/autopilot.ts` (`openingActions()` and the auto-trigger path).
- Unaffected: `experienceFromBank()`'s base-CV (vacancy-less) seeding behavior; the résumé-less 409;
  the Score/ATS-delta tab; `/my/activity/matches`; `/my/profile`'s `ATSReportView`.
