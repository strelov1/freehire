## Why

`ResolveWithDrafting` (`internal/api/atsapply/draft.go`) offers every required, unmapped,
non-sensitive field `LLMDrafter.Draft` could answer — both free-text fields (`text`,
`textarea`) and single-choice `select` fields — through one generative LLM call per field
(`internal/api/atsapply/llm_drafter.go`). For `select` fields this is the same discrete
"pick one from a closed list" task that `autofill-choose-jev-provider` already moved onto
Typesafe AI's Jev for the browser extension's `Choose` call, at a fraction of the latency
and without JSON-parsing/hallucination risk, since Jev's answer is constrained to the
offered criteria.

A feasibility spike (2026-10-09) replayed 6 realistic auto-apply questions directly
against the live Jev API and got 6/6 matches. Two of the six deliberately tested the
riskiest unknown first: `draftSystemPrompt`'s categorical exclusion ("never answer
identity/demographics/compensation/legal work status even if the question seems
answerable") is a different decline mechanic than "nothing supports any option" — it must
decline even when a candidate fact looks superficially supportive. Both adversarial cases
(a stated salary negotiation range against a compensation question; stated years working
on-site in the US against a legal work authorization question) declined correctly. This
proposal acts on that validated result for `select`-kind fields only.

## What Changes

- Add a `JevSelectDrafter` that wraps the existing `LLMDrafter`: `text`/`textarea` fields
  are delegated unchanged to `LLMDrafter.Draft` (open-ended generation Jev cannot do);
  `select`-kind fields answer via Jev's `/v1/systemone` `Choice` question (the field's
  options as criteria, plus an explicit decline alternative covering both "nothing
  supports an option" and "this question falls in a categorically excluded topic") when
  `TYPESAFE_API_KEY` is configured.
- `Draft` falls back to `LLMDrafter.Draft` whenever Jev errors (network failure, bad key,
  malformed response), so a third-party outage degrades to today's behavior rather than
  leaving the field unmapped that the LLM could have answered.
- When `TYPESAFE_API_KEY` is unset, the new drafter behaves exactly like `LLMDrafter`
  alone — Jev is never called.
- Wire the new drafter in at `cmd/auto-apply`'s construction of the `Drafter` passed to
  `ResolveWithDrafting`.
- Reuse the `TYPESAFE_API_KEY` setting `autofill-choose-jev-provider` already introduced
  — no new config knob.
- **BREAKING**: none. The output contract (`answer string, ok bool, err error`, with
  `matchOption` still re-validating the answer against the field's own offered options
  downstream) is unchanged; only the backend answering `select` fields differs when
  configured.

## Capabilities

### New Capabilities
- `atsapply-select-drafter-provider`: governs which backend answers a `select`-kind
  draftable field (Jev when configured, LLM otherwise or on Jev failure), the categorical
  decline requirement for identity/demographics/compensation/legal-status questions, and
  the fallback semantics that must hold regardless of which backend answers.

### Modified Capabilities
(none — `LLMDrafter.Draft` for `text`/`textarea`, `draftable`'s gating, `matchOption`'s
re-validation, and every other `atsapply` capability are unchanged)

## Impact

- **Code**: `internal/api/atsapply/` (new `JevSelectDrafter` + reuse of the Jev HTTP client
  shape from `internal/ai/autofillagent/jev.go` — a small, package-local client, not a
  shared one, matching that package's own "no new provider abstraction in
  `internal/platform/llm`" decision), `cmd/auto-apply/main.go` (construction-site swap).
- **Dependencies**: none added — same hand-rolled HTTP client approach as
  `autofill-choose-jev-provider`, for the same reason (no trustworthy external Go client
  exists for Jev).
- **Out of scope**: `LLMDrafter.Draft`'s `text`/`textarea` path, `cmd/enrich`,
  `autofillagent.LLMPlanner.Plan` — none of that is touched here.
