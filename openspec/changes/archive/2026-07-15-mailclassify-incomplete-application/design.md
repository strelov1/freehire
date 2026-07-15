## Context

`StatusSignal` is the controlled vocabulary an email is classified into; unknown
values coerce to `other` (`Sanitize`). Confident status skips the LLM via
`KeywordStatus` for auto-linked emails. `signalStage` maps a signal to the stage
it advances to; negative/non-progress signals are absent (never auto-advance).

## Goals / Non-Goals

**Goals:** capture the actionable "you did not finish applying" case as its own
signal; keep it on the deterministic fast-path where possible; do not let it
touch the application stage.

**Non-Goals:** the descriptive interview-lifecycle split (invite/follow-up/
feedback) — deliberately not added (no action attached). No schema change.

## Decisions

- **Non-advancing.** `incomplete_application` is left OUT of `signalStage`: an
  unfinished application has not reached `applied`, so it must not move the stage.
- **Keyword rule, ordered before acknowledgement.** Its phrases ("complete your
  application", "application is incomplete", "finish your application", "action
  required to complete") are templated and unambiguous; ordering it before the
  acknowledgement templates prevents an "…thank you for starting your application,
  please complete it…" email from resolving to acknowledgement.

## Risks / Trade-offs

- Minor overlap with `info_request` (both ask the user to act); the keyword rule
  targets the "incomplete/complete your application" phrasing specifically, and
  the ambiguous remainder still defers to the LLM.
