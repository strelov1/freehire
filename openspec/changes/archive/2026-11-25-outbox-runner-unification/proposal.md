## Why

Six independent Postgres-backed work queues (`enrichment_outbox`, `semantic_outbox`,
`search_outbox`, `apply_form_outbox`, `adzuna_description_outbox`,
`email_classification_outbox`) each drive their own worker package
(`internal/enrich`, `internal/embed`, `internal/searchdrain`, `internal/applyform`,
`internal/adzunadesc`, `internal/maillink`) with a hand-rolled claim/lease/retry drain
loop. The six loops are near-identical by construction — each was deliberately copied
from the previous one (SQL comments literally say "Mirrors ClaimSemanticBatch",
"Mirrors ClaimApplyFormBatch") — so the duplication is boilerplate, not divergent
design. A seventh queue would mean copying the loop an seventh time. Full design
rationale, alternatives considered, and API shape: see
`docs/superpowers/specs/2026-08-09-outbox-runner-unification-design.md`.

## What Changes

- Add a new `internal/outbox` package: a generic (Go type-parameter) claim-wave
  runner with two entry points — `RunPool[C any]` (bounded-concurrency per item, for
  workers whose cost is per-call regardless of batching) and `RunBatch[C any]`
  (one call per wave with per-item fallback, for workers that batch cheaply).
- Migrate all six existing workers (`internal/enrich`, `internal/embed`,
  `internal/searchdrain`, `internal/applyform`, `internal/adzunadesc`,
  `internal/maillink`) onto `internal/outbox`, replacing each package's own
  hand-rolled claim-wave loop. Each package's `Store`/`Indexer`/`Provider` port and
  its `Complete`/`Save`/`Fail`/`Discard` logic are untouched — only the outer loop
  driving them changes.
- Shrink each of the six existing `runner_test.go` files to cover only
  package-specific logic (batch/fallback boundary, `Sanitize`/`Validate` wiring,
  `ErrPostingGone` → `Discard`, sequential-ordering requirement, etc.); loop-mechanics
  tests (claim-until-empty, `MaxPerRun` budget, context cancellation) move to a new
  `internal/outbox/runner_test.go`, written once instead of six times.
- No **BREAKING** changes: no schema migration, no sqlc query change, no
  `cmd/<name>` binary rename, no env var change, no systemd unit change, no change
  to any worker's external behavior or exit-code convention
  (`worker.ExitCode(failed, deadLettered)` keeps working unchanged).

## Capabilities

### New Capabilities
(none — `internal/outbox` is an internal implementation detail with no
spec-level/user-facing behavior of its own)

### Modified Capabilities
(none — this is a pure internal refactor; every worker's external behavior,
retry/dead-letter semantics, and API/DB contract stay exactly as they are today)

## Impact

- **New**: `internal/outbox` (package + tests).
- **Changed**: `internal/enrich`, `internal/embed`, `internal/searchdrain`,
  `internal/applyform`, `internal/adzunadesc`, `internal/maillink` — each package's
  `runner.go` (or equivalent) swaps its own loop for a call into `internal/outbox`;
  each package's `runner_test.go` shrinks accordingly.
- **Unaffected**: all six `cmd/<name>` binaries (same names, same flags, same env
  vars), all six DB tables and their sqlc queries, all six `Store`/`Indexer`/
  `Provider` port interfaces and their `Complete`/`Save`/`Fail`/`Discard` methods,
  `internal/worker.ExitCode`.
