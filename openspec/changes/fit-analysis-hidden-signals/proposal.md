## Why

The AI fit analysis already tells a candidate whether their CV covers a posting's stated
requirements, but a job posting also carries unstated expectations — pace, ownership, team
stage — that live in word choice rather than in any explicit requirement line. Today none of
that is surfaced, so a candidate reads the same requirement table for a calm, structured
0-to-1 team and a scrappy, ambiguity-heavy one and gets no signal about which they're walking
into.

## What Changes

- Extend the existing Stage 1 (Extract & Match) LLM call in `internal/matchanalysis` to also
  return up to 5 "hidden signals" — each a short verbatim quote from the job description plus
  a one-line interpretation of what it implies about pace, ownership expectations, team stage,
  or culture. No new model call, no new stage.
- Sanitize the new output the same way the rest of Stage 1's untrusted output is handled today
  (drop blank entries, bound field lengths, cap the count) before it is persisted or served.
- Thread the sanitized signals through the existing pipeline into the served/cached `Analysis`
  wire struct, alongside `RequirementMatch`, using the existing triple-stamp cache with no new
  invalidation rule.
- Stream the signals to the frontend via the existing Stage-1 SSE event, so they appear at the
  same moment the requirement match does, before Stage 2/3 finish.
- Render a new section on the fit analysis page showing the quote + interpretation pairs; the
  section is omitted entirely when the model returns zero signals (a generic/uninformative
  posting is not forced to produce filler).
- Regenerate the TS wire contract for the new field via `cmd/gen-contracts`.

## Capabilities

### New Capabilities

(none — this extends the existing fit-analysis capability rather than introducing a new one)

### Modified Capabilities

- `job-fit-analysis`: the served analysis payload gains a `hidden_signals` field (0-5
  quote+interpretation pairs) produced and sanitized as part of the existing Stage 1 extraction,
  with no new endpoint, model call, or cache-invalidation rule.

## Impact

- `internal/matchanalysis/matchanalysis.go` — new `Signal` type, `HiddenSignals` field on
  `Analysis` and `stage1Out`, a `sanitizeSignals` function, new bound constants.
- `internal/matchanalysis/analyzer.go` — Stage 1 system prompt gains the hidden-signals
  instruction block; `stage1Out` decode picks up the new key; the Stage-1 SSE event payload
  carries the sanitized signals.
- `cmd/gen-contracts` output (generated TS types) regenerated for the new field.
- `web/src/lib/matchAnalysis.ts` — SSE reducer carries `hidden_signals` through from the
  Stage-1 event onward.
- `web/src/routes/match/[slug]/` — new UI section rendering the signals, empty-state omitted.
- No new migration, no new endpoint, no new background worker.
