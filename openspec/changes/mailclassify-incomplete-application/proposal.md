## Why

Application emails include a distinct, actionable case the current vocabulary
misses: the application was **started but not finished** ("your application is
incomplete", "action required to complete your application"). Today these fall
into `info_request` or `other`, burying a genuine to-do. Unlike the finer
interview-lifecycle labels (invite/follow-up/feedback), which are descriptive
only, "incomplete application" tells the user to *do* something.

## What Changes

- Add an `incomplete_application` status signal to the controlled vocabulary,
  the LLM prompt, and the deterministic keyword fast-path (its language is
  templated, so it stays off the LLM). It does NOT auto-advance the application
  stage — an unfinished application has not progressed.
- Surface it in the inbox status badge.

## Capabilities

### Modified Capabilities
- `email-body-classification`: add `incomplete_application` to the classified
  status vocabulary.

## Impact

- `internal/mailclassify/classification.go` (vocab + Sanitize), `classifier.go`
  (prompt), `keyword.go` (fast-path rule).
- `web/src/lib/emailStatus.ts` (badge label + colour).
- No stage-mapping change (it is intentionally non-advancing). Existing emails
  keep their labels; a re-classification applies it to history.
