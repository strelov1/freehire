## Context

`internal/worker/resilient.go` is the shared answer to an observed production condition:
a damaged TOAST pointer makes a whole `SELECT *` page fail with SQLSTATE `XX001`, so a
full-table scan dies on one bad row. `ResilientPage` degrades — re-lists the window as
bare ids (a projection that never detoasts), fetches rows singly, skips the ones that
still fault, and advances the cursor past them.

Three workers scan the whole `jobs` table. One uses it:

| Worker | Scan | Resilient? | On `XX001` |
|---|---|---|---|
| `cmd/reindex` | `NewFullScanReader` + `ResilientPage` | yes | skips, logs, completes the swap |
| `cmd/backfill-derive` | raw `ListJobsByIDAfter` in the producer goroutine | no | `fail(e)` cancels the run |
| `cmd/backfill-descriptions` | raw `ListJobsByIDAfter` via `pageJobs` | no | aborts `backfillAll` |

`cmd/backfill-derive` has no resume flag, so its failure mode is not "this run failed" but
"this worker can never finish again": each run restarts from id 0 and dies at the same row.

## Goals / Non-Goals

**Goals:**

- Make the whole-catalogue derive pass finishable in the presence of a corrupted row.
- Reuse the existing helper rather than growing a second degrade path.

**Non-Goals:**

- `cmd/backfill-descriptions`. Its scoped branch pages by source, which no existing
  `PageReader` constructor serves, and it is a one-off historical encoding repair. A third
  constructor built for it would be infrastructure ahead of need.
- A resume flag for `cmd/backfill-derive`. Worth considering separately; skipping the
  unreadable row removes the reason it is currently needed.
- Changing what the backfill derives, its concurrency, or its progress reporting.

## Decisions

### D1: `NewFullScanReader` narrows from `jobQueries` to the three methods it calls

`jobQueries` declares five methods because it serves *both* reader constructors —
`fullScanReader` uses `ListJobsByIDAfter` / `ListJobIDsAfter` / `GetJob`, and
`postedSinceReader` uses the two `ListOpenJobs*PostedAfter`. `NewFullScanReader` therefore
demands two methods it never invokes.

That is harmless while the only caller passes `*db.Queries`, which has everything. It stops
being harmless the moment a worker wants to pass a narrower store: `cmd/backfill-derive`'s
`deriveStore`, and the fake in its tests, would each have to declare two dead
`ListOpenJobs*PostedAfter` methods to satisfy a constructor that ignores them.

So each constructor takes its own narrow interface. `*db.Queries` satisfies both, and
`cmd/reindex` is untouched.

*Alternative considered:* have `cmd/backfill-derive` implement `worker.PageReader` itself
over its own store. Rejected — it re-implements the adapter `NewFullScanReader` already is,
which is the duplication this change exists to remove.

*Alternative considered:* widen `deriveStore` to all five methods. Rejected — it puts two
permanently-unused methods on the production store interface and on every test fake, to
work around a declared dependency that was too wide to begin with.

### D2: The exhaustion test moves to the keyset cursor

The current loop ends on two conditions: an empty page, and a page shorter than
`backfillBatchSize`. The second is an optimisation that is *wrong* under the degrade path —
a page that skipped a corrupted row is legitimately short, and treating it as the end of
the table would stop the scan at the first corrupted row while reporting success. That is
strictly worse than today's hard failure: a silent partial pass over a catalogue-wide
re-derive leaves stale columns with nothing to indicate it.

`ResilientPage` returns `lastID == afterID` — no cursor movement — precisely as its
exhaustion signal, and `cmd/reindex/main.go:355-358` already carries this reasoning in a
comment. The backfill adopts the same test.

### D3: Skipped ids are counted and logged, not aggregated into the run's error

A skipped row is a fact about the database, not a failure of the pass. `ResilientPage`
already logs each id; the producer accumulates the count so the run's summary can report
it, matching how `cmd/reindex` reports `skipped`.

## Risks / Trade-offs

**A corrupted row now leaves its derived columns stale silently, where before the run
failed loudly.** → It did not fail *usefully*: with no resume flag, the loud failure meant
the entire catalogue past that id stayed stale, not just the one row. The skipped ids are
logged and counted in the summary, so the signal is preserved and the blast radius shrinks
from "everything after the bad row" to "the bad row".

**Narrowing an exported constructor's parameter type is an API change to
`internal/worker`.** → Internal package, one non-test caller, and it narrows rather than
widens: every existing argument still satisfies it, so nothing outside can break.

**The degrade path issues one query per row for the faulting window.** → Bounded by one
batch, only on a batch that actually faulted, and only for that window. The alternative is
not scanning at all.

## Migration Plan

None. No schema, no API, no wire shape. The worker is run-once-and-exit and cron-driven, so
the change takes effect on its next run. Rollback is a plain revert.

## Open Questions

None.
