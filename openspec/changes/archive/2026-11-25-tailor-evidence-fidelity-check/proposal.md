## Why

The evidence gate (`cv-tailoring`'s "must cite evidence" requirement) checks that a claim-shaped
edit cites a real, candidate-asserted atom — but it never checks that the WORDING of that edit
stays inside what the cited atom actually says. An agent can cite a real evidence id and still
write a bullet that claims more scope, seniority, or a bigger metric than the atom supports; the
tailoring prompt already states the rule ("stay inside what the evidence says... never invent,
inflate or imply"), but nothing checks it, so it can silently be violated. A throwaway spike
(`/tmp/crewai-spike/SPIKE_RESULTS.md`, 2026-08-09) reproduced exactly this failure across three
CV/JD pairs with a plain single-pass tailor, and validated that a second look catches it reliably
— while also showing a separate agent framework (CrewAI, a second service) is not needed to get
that benefit; one bounded extra step in the same agent does.

## What Changes

- A new tool for the tailoring agent (`internal/handler/assistant_cv_tools.go`, alongside
  `cv_edit`/`job_match`) that, given an `evidence_id`, returns that atom's own `claim`/`context`/
  `metrics` — the same fields the agent already saw before writing, surfaced again as a forced
  checkpoint rather than as new information.
- A `tailorPrompt` instruction: after a batch of edits that cited evidence, call this check for
  each cited id and compare the wording just written against what the atom actually says; revise
  via `cv_edit` if the bullet claims more than the atom supports.
- Bounded the same way the existing `job_match` self-check is bounded: a soft cap (2-3 rounds),
  the agent's own judgment to stop, no numeric gate — fidelity is a semantic question a
  deterministic check cannot arbitrate, so this stays advisory/self-directed rather than a
  second server-side refusal.
- No new capability, no new service, no CrewAI: the check is one more tool call inside the
  existing single agent loop, consistent with how the `job_match` self-check was designed.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `cv-tailoring`: adds a requirement that a claim-shaped edit citing evidence can be checked, by
  the agent itself, for fidelity to that evidence's own wording — extending the existing
  evidence-citation requirement's honesty rule from "cites something real" to "stays inside what
  it cites."

## Impact

- `internal/handler/assistant_cv_tools.go` — new tool alongside `cv_edit`/`job_match`; reuses the
  existing `bank.GetAtom` read path already used by `bankGate.Publishable`, no new data layer.
- `internal/assistant/prompt.go` — `tailorPrompt` gains the check-and-revise instruction.
- Both the conversational and autopilot rhythms of tailoring pick this up for free, since both
  run the same `tailorPrompt` (`tailor-autopilot`'s own spec: "the same method the conversational
  tailoring uses — the rhythm differs, not the rules").
- Tests: extends the existing scripted-model integration coverage for `internal/handler`'s
  assistant CV tools (`//go:build integration`).
