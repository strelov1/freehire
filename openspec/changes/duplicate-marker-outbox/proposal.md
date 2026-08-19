## Why

A posting stops being a duplicate — or becomes one — the moment a dedup pass writes its
marker. It leaves or re-enters the facet index only at the next full rebuild, up to six hours
later. For those six hours search shows a repost as its own vacancy, or hides a posting that
is canonical again.

Every other change already travels incrementally. New and edited postings go through
`search_outbox`; closed ones through `search_delete_outbox`, shipped 2026-08-18. Duplicate
markers are the last thing the rebuild is the only path for.

They were left out for a reason that no longer holds. Until 2026-08-19 a marker refresh
changed ~950,000 rows per cycle, six cycles a day, because the three passes overwrote each
other's verdicts. At the drain's measured 200 documents per ~7.7s that is over ten hours of
queue per cycle — the queue would have grown faster than it drained. Ownership
(`duplicate-marker-ownership`) took a cycle to **13,385** changed rows, about 67 batches, a
few minutes. The blocker is gone.

## What Changes

- Each of the three marker passes enqueues the rows whose duplicate status actually flipped,
  in the same statement that flips it:
  - canonical → duplicate: the document must go, so the row is queued on
    `search_delete_outbox`.
  - duplicate → canonical: the document must come back, so the row is queued on
    `search_outbox`.
  - a duplicate that merely changes which canon it points at: nothing. Its document is
    already absent, and re-queueing it would be work with no effect.
- The transition is read from the DERIVED `jobs.duplicate_of`, not from the pass's own
  column. A pass clearing its own marker does not make the row canonical if another pass
  still holds one — reading its own column would put a still-duplicate posting back into
  search.
- `MarkJobDuplicateOfRole`, the ingest-time role verdict, gets the same treatment for the
  same reason.
- **Nothing is backfilled.** Duplicates already sitting in the index have no transition to
  detect; the scheduled rebuild continues to collapse them, exactly as today. This change
  bounds how long a NEW duplicate lingers, and deliberately does not try to be the rebuild.

## Capabilities

### New Capabilities

- `duplicate-marker-indexing`: how a change in duplicate status reaches the facet index —
  which transition queues which way, which value the decision reads, and what is deliberately
  left to the rebuild.

### Modified Capabilities

None. The queues' own contracts (`ClaimSearchOutboxBatch` requires open and canonical;
`ClaimSearchDeleteOutboxBatch` needs only a primary key) are unchanged — this change only
gives them a new producer.

## Impact

- **Queries:** `internal/db/queries/jobs.sql` — `RecomputeRoleDuplicatesForCompanies`,
  `SuppressAggregatorDuplicatesForCompanies`, `MarkFuzzyDuplicatesForCompany`,
  `MarkJobDuplicateOfRole`. Each gains a CTE over `RETURNING` in the shape the closing
  statements already use for `search_delete_outbox`.
- **No version floor and no new dependency.** The transition is read from the statement's own
  snapshot: a CTE selecting `duplicate_of` holds the value before the update, `RETURNING`
  holds it after, and every CTE of one statement sees the same snapshot. This began as
  Postgres 18's `RETURNING old./new.` — verified working, prod is 18.4 — until sqlc 1.31.1
  rejected the syntax; the snapshot form is portable and no larger.
- **Return types:** the three batch passes are `:execrows`; a CTE moves the row count out of
  the command tag, so they become `:one` ending in `SELECT count(*)`, as PR #2133 did for the
  five closing statements. Callers keep taking `int64`.
- **Not affected:** the drain, the queue schemas, the claim predicates, the rebuild, and
  every reader of `duplicate_of`.
- **Volume:** ~13,385 rows a cycle across all three passes today, of which only the flips
  queue — the rest change canon without changing status.
