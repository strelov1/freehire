## Why

`cmd/reindex`'s full facet rebuild never touches `search_outbox`, even though it makes
that queue's pending entries provably stale in bulk. A full reindex reads every job's
CURRENT content straight from Postgres and swaps in a brand-new live index — so any
`search_outbox` row queued *before* that run started is redundant the instant the swap
succeeds: the job's current content is already in the freshly-built index.
`cmd/search-drain` doesn't know this and will re-push that same content again for no
reason, burning the queue's (expensive — see the already-merged `proximityPrecision`
fix, PR #1637) capacity on pure duplicate work. Discovered operationally: `search_outbox`
was observed backlogged to ~90-113k pending entries in production even while
search-drain ran continuously and error-free.

## What Changes

- `cmd/reindex` captures the run's start time before it does anything else.
- After a **full, unscoped facet rebuild** (`cmd/reindex`'s default target — the only
  path search_outbox is even relevant to; the semantic index has its own separate
  `semantic_outbox`) successfully swaps in (`Promote()` returns without error), delete
  every `search_outbox` row whose `created_at` **and** its job's `jobs.updated_at` are
  both strictly before that captured start time (see design.md Decision 1a for why
  `created_at` alone isn't enough — `ON CONFLICT DO NOTHING` can leave it stale).
- Rows queued **during** the run are left untouched — some represent a job that changed
  again after the reindex's scan already passed its row, so a future search-drain run
  still needs them to catch up.
- A best-effort step: a failure to purge logs and does not fail the reindex run (the
  reindex itself already succeeded; a missed purge just means those rows survive one
  extra, harmless cycle).
- New sqlc query `DeleteSearchOutboxCreatedBefore` in
  `internal/db/queries/search_outbox.sql` (no migration — `search_outbox.created_at`
  already exists).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — this is an internal efficiency fix to the reindex worker's own pipeline, not a
change to any capability's request/response contract or observable behavior other than
a smaller queue backlog.)

## Impact

- `internal/db/queries/search_outbox.sql` + regenerated `internal/db` (`make sqlc`).
- `cmd/reindex/main.go` — `run()` captures a start timestamp; the facet-swap branch
  calls the new purge query after a successful `Promote()`.
- `cmd/search-drain` (internal/searchdrain) — sees a smaller backlog after every full
  reindex (currently every 3h on prod's normal schedule) at no cost to it; no code
  changes needed there.
- No schema, API, or migration changes.
