## Why

`worker-lifecycle` already requires it: *"A worker process SHALL exit with a non-zero code when
its run completes with one or more per-item failures or dead-lettered items."* Eight binaries
obey it. `cmd/classify-mail` logged `"classify-mail: done"` and returned `0` unconditionally.

It could not do otherwise. The information was dropped three layers down:
`FailEmailClassification` was `:exec`, where its two siblings — `RecordEnrichmentFailure` and
`RecordSemanticFailure`, which its own comment claims to mirror — are `:one ... RETURNING
attempts, failed_at`. So `maillink.Store.Fail` returned only `error`, the `Run` loop kept no
tallies, and `main` had nothing to report.

Nothing else reads `email_classification_outbox.failed_at`. A mail queue that dead-letters every
message was therefore visible in `journalctl` and nowhere else — which is exactly the state
`worker.ExitCode` exists to surface.

## What Changes

- `FailEmailClassification` becomes `:one ... RETURNING attempts, failed_at`, matching the two
  queries its comment already said it mirrored.
- `maillink.Store.Fail` returns `(deadLettered bool, err error)`; `Runner.Run` returns
  `Stats{Failed, DeadLettered}`, tallied where it already logged the bookkeeping error.
- `cmd/classify-mail` ends on `worker.ExitCode(stats.Failed, stats.DeadLettered)` and logs the
  two counts.
- **A bookkeeping write that itself fails does not guess.** The entry is then left to its lease
  expiry, so its dead-letter state is unknown: it counts as failed and not as dead-lettered.
- Deliberately NOT extracted: a shared `worker.Outcome` for two callers of a dozen lines each.
  `enrich` tallies under a mutex because a wave runs concurrently; `embed` deliberately fails on
  `context.Background()`. They are not the same shape.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `worker-lifecycle`: "Bookkeeping failures are logged and counted" was scoped to *the enrichment
  drain*. It binds every queue drain — leaving it enrichment-specific is how the next one drifts —
  and it gains the rule that a drain must not guess a dead-letter it could not record.

## Impact

- `internal/db/queries/mail_classification.sql` (+ regenerated `internal/db`),
  `internal/maillink/runner.go`, `cmd/classify-mail/{main,store}.go`.
- No migration: the column already exists and the statement already wrote it. Only the read is new.
- Operationally visible: a classify-mail run with any failure now exits non-zero, so cron alerts
  where it previously did not.
