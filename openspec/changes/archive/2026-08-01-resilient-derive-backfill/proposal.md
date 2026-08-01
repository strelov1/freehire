## Why

`worker.ResilientPage` exists because of an observed production condition: one damaged
TOAST pointer fails an entire `SELECT *` page, and commit `1153d215` ("survive corrupted
(XX001) rows in full-scan workers") records that this already crashed a full facet reindex
on this database. The shared answer was written, tested, given two reader constructors —
and then wired into exactly one of the three workers that scan the whole `jobs` table.

`cmd/backfill-derive` is the one that needs it most. CLAUDE.md describes it as the worker
that "re-derives every deterministic column (facets, `role_fingerprint`, slugs) in one
keyset pass" — a whole-catalogue pass is its entire purpose. Its producer goroutine issues
a raw `ListJobsByIDAfter`, and any error calls `fail(e)`, which cancels the run. It has no
resume flag. So a single corrupted row makes the whole-catalogue re-derive **permanently
unable to finish past that id**: every run re-fails at the same place, and every column
after it stays stale forever.

## What Changes

- `cmd/backfill-derive`'s producer reads through `worker.ResilientPage` over a
  `worker.NewFullScanReader`, so a corrupted row is skipped and logged instead of ending
  the run.
- Its exhaustion test changes from "the page was shorter than the batch" to "the keyset
  cursor did not advance". This is a correctness requirement of the move, not a tidy-up:
  the degrade path legitimately returns a short page after skipping a damaged row, so the
  old test would stop the scan at the first corrupted row — converting a hard failure into
  a silent partial pass, which is worse.
- `deriveStore` widens from two methods to four: the three the full-scan reader calls plus
  the update it already had.
- `worker.NewFullScanReader`'s parameter narrows from the five-method `jobQueries` to the
  three methods it actually calls. Without this the backfill's store — and its test fake —
  would have to declare two `ListOpenJobs*PostedAfter` methods that the full-scan reader
  never invokes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `corruption-resilience`: adds a requirement that the whole-catalogue derive backfill
  completes despite corrupted rows, alongside the existing one for `reindex`.

## Impact

- `cmd/backfill-derive/main.go` — the producer loop and the store interface.
- `cmd/backfill-derive/main_test.go` — the fake gains the two reader methods, and gains a
  case for the corrupted-row path.
- `internal/worker/resilient.go` — `NewFullScanReader`'s parameter type narrows.
  `*db.Queries` satisfies both, so `cmd/reindex` is unaffected.

No schema, API or wire-shape change. No migration.

### Deliberately out of scope

`cmd/backfill-descriptions` has the same raw scan. Its scoped branch pages by source, which
`NewFullScanReader` does not serve, and inventing a third `PageReader` constructor for a
one-off historical encoding repair is infrastructure ahead of need. Its unscoped branch
could adopt the existing constructor later if that repair is ever re-run.
