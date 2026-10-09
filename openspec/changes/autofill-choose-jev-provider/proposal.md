## Why

The browser-extension autofill agent answers one combobox question at a time via
`autofillagent.LLMPlanner.Choose` (`internal/ai/autofillagent/planner.go:86`): given a
question, the exact options the page offers, and the candidate's profile, it picks one
option verbatim or declines. This is a closed, discrete choice — exactly the task
Typesafe AI's Jev model (`System-One`, `Choice` question type) is built for, at a fraction
of a generative LLM's latency (~0.3-0.5s observed vs. multi-second LLM calls) and without
JSON-parsing or hallucination risk, since Jev's answer is constrained to the options given.

A feasibility spike (2026-10-08) replayed 8 realistic cases drawn from `Choose`'s own
documented rules (location, visa sponsorship, relocation, notice period, salary-range, and
two "nothing supports any option" decline cases) directly against the live Jev API and
got 8/8 matches, each at 0.99-1.0 confidence, including both decline cases. This proposal
acts on that validated result for `Choose` only.

## What Changes

- Add a `JevPlanner` that wraps the existing `LLMPlanner`: `Plan` is delegated unchanged
  (it mixes free-text generation with discrete choice in one call, which does not fit
  Jev's `Choice`/`Noul` question shape); `Choose` answers via Jev's `/v1/systemone`
  `Choice` question (closed options plus an explicit "none of these" alternative) when
  `TYPESAFE_API_KEY` is configured.
- `Choose` falls back to `LLMPlanner.Choose` whenever Jev errors (network failure, bad
  key, malformed response), so a third-party outage degrades to today's behavior rather
  than leaving the field unanswered.
- When `TYPESAFE_API_KEY` is unset, `JevPlanner` behaves exactly like today's
  `LLMPlanner` — Jev is never called.
- Wire `JevPlanner` in at the one construction site that currently builds `LLMPlanner`
  for the autofill agent (`internal/api/handler/autofill_agent.go:48`).
- **BREAKING**: none. The output contract (a verbatim option from the offered list, or
  empty for decline) is unchanged; only the backend answering it differs when configured.

## Capabilities

### New Capabilities
- `autofill-choice-provider`: governs which backend answers a combobox `Choose` question
  (Jev when configured, LLM otherwise or on Jev failure) and the fallback/decline
  semantics that must hold regardless of which backend answers.

### Modified Capabilities
(none — `Plan`'s behavior, the extension-autofill contact block, and every other
autofill-agent capability are unchanged)

## Impact

- **Code**: `internal/ai/autofillagent/` (new `JevPlanner` + Jev HTTP client),
  `internal/api/handler/autofill_agent.go` (one-line construction swap),
  `internal/platform/config/` (new optional `TYPESAFE_API_KEY` setting).
- **Dependencies**: none added. `https://api.typesafe.ai/v1/systemone` (confirmed working
  with a live key during the spike) is called through a minimal hand-rolled HTTP client in
  `internal/ai/autofillagent/jev.go` rather than a third-party Jev SDK — see design.md for
  why the one existing Go wrapper was rejected.
- **Out of scope**: `LLMPlanner.Plan`, `internal/api/atsapply/llm_drafter.go`
  (`LLMDrafter.Draft`), `cmd/auto-apply`, `cmd/enrich` — each mixes free-text generation
  with discrete choice in a single call and needs its own refactor before Jev fits; none
  of that is touched here. A prior attempt to add Jev to `cmd/enrich` (PR #3068) was
  closed because it replaced ~19 enrichment fields with a destructive write — not
  applicable here, since this change writes nothing to `jobs.enrichment` or any stored
  blob.
