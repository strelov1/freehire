## Why

The Tailor autopilot run computes the fit analysis once, as a precondition before it starts, and
never checks it again — the agent finishes a run trusting its own judgment that an edit closed a
gap, with no independent confirmation that the deterministic scoring logic reads it the same way.
Separately, the cached fit analysis is deliberately frozen as "a snapshot of the base profile" so
it is never mixed with the live `job_match` score — a rule written for a product moment (deciding
whether to bother tailoring, from the job listing) that cold-start autopilot already removed:
opening Tailor now starts editing immediately, so there is no "before" number worth freezing
anymore.

## What Changes

- The autopilot run, before reporting, calls the existing free `job_match` tool to check whether
  its edits actually closed what it believes they closed, and keeps editing (soft cap ~2-3
  rounds) while a closeable gap remains.
- The server unconditionally recomputes and overwrites the cached `(user, job)` fit analysis once
  every autopilot run ends — even when the run made zero edits. This repeals the rule that the run
  must leave the fit analysis untouched.
- The cached fit analysis is no longer described as a frozen "snapshot of the base profile" — it
  is now kept current by the latest autopilot run.
- **BREAKING (behavioral, not API-shape):** a candidate who reopens a vacancy's fit analysis
  outside Tailor (e.g. the job listing drawer) after tailoring a CV for it will now see that
  tailoring's result, not their pre-tailoring baseline. No response field changes.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `tailor-autopilot`: a run now calls `job_match` and may keep editing before it reports (new
  requirement), and the fit analysis it sits above is now recomputed after the run rather than
  left alone (flips the existing "left alone" requirement and scenario).
- `tailor-job-match`: the fit analysis is no longer labelled as a base-profile snapshot; it is
  labelled as kept current by the latest autopilot run.
- `tailor-workspace`: the Job Match tab's description of its second number (the cached fit
  analysis) is updated to match — no longer "a snapshot of the base profile."

## Impact

- `internal/handler/match_analysis.go`: extract `buildAnalysisInput` from `runAnalysis`; add
  `prepareAutopilotRun`.
- `internal/handler/assistant.go`: `PostAssistantAutopilot` calls the post-run refresh after the
  turn completes.
- `internal/assistant/prompt.go`: the Tailor preset's `UNATTENDED RUNS` section gets the
  self-check instruction.
- `web/src/lib/tailor/ArtifactPanel.svelte`: Job Match tab copy no longer calls the fit analysis a
  snapshot.
- No DB migration — still one `user_job_analysis` row per `(user, job)`.
- Out of scope, deliberately: restructuring `internal/matchanalysis`'s three-stage chain itself.
  Full design rationale: `docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md`.
