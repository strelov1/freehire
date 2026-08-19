## Context

Three passes write duplicate markers, each into its own column since
`duplicate-marker-ownership`; `jobs.duplicate_of` is derived from them by a BEFORE trigger
(migration 0115) and is what every reader consumes. `ClaimSearchOutboxBatch` requires a job to
be open AND canonical, so a posting that becomes a duplicate is simply never re-indexed — its
existing document persists until a rebuild swaps the whole index.
`ClaimSearchDeleteOutboxBatch` needs only a primary key and is already drained beside it.

So the mechanism to remove a document incrementally exists and works; nothing produces the
removals for duplicates. Producing them is the whole change.

## Goals / Non-Goals

**Goals:**

- A posting that becomes a duplicate leaves search in minutes, not up to six hours.
- A posting that becomes canonical returns just as fast.
- The decision is made in the same statement that writes the marker, so an interrupted run
  cannot leave a status change unqueued.
- No new queue, no new worker, no schema change.

**Non-Goals:**

- Changing the rebuild's schedule. That is a separate decision that this change makes
  *possible* to consider, and deliberately does not take.
- Backfilling the duplicates already in the index. They have no transition to observe and the
  rebuild still collapses them.
- Touching the drain, the claim predicates, or the queue tables.

## Decisions

### Decision 1: Read the transition from one statement's own snapshot

Every CTE of a single statement reads the same snapshot, so a CTE that selects `duplicate_of`
alongside the pass's own work holds the value as it stood BEFORE the update, while the
UPDATE's `RETURNING` holds it after. Both in one statement, no window between them.

The role and aggregator passes already read `jobs` in their `target` CTE to compute the new
marker, so the old value comes free — one more column. The fuzzy pass and
`MarkJobDuplicateOfRole` drive off an argument rather than a scan, so they gain a small
`before` CTE that reads only the rows they are about to touch.

*This started as `RETURNING old.duplicate_of, new.duplicate_of`* — Postgres 18 syntax, and
prod is 18.4. It was verified working against `pgvector/pgvector:pg18`, including the part
that mattered: `new.` reflects the BEFORE trigger's `COALESCE` rather than the value assigned
to an owned column. Then sqlc 1.31.1 rejected it — its analyzer predates PG18 — and the
snapshot form turned out to be both portable and no more code. The verification was not
wasted: it established that the transition is readable atomically at all, which is the
property the design needs; the syntax was only the first way found to get it.

*Alternative — read the current markers in one statement, then write in another:* a window in
which another pass can change the same row, so the recorded "before" may not be the value the
write replaced. The failure is silent and directional: a missed removal leaves a duplicate in
search.

*Alternative — a trigger that enqueues:* moves the logic away from the statement that owns it
and fires on every marker write including the backfill's, which would have queued two million
rows. Rejected.

### Decision 2: Branch on the DERIVED marker, never on the owning column

`old.duplicate_of IS NULL AND new.duplicate_of IS NOT NULL` → queue a removal.
`old.duplicate_of IS NOT NULL AND new.duplicate_of IS NULL` → queue an index.
Both non-NULL, or both NULL → queue nothing.

Branching on the pass's own column would be wrong in exactly the case ownership created: the
aggregator pass releasing a suppression on a posting the role pass still marks. Its own column
goes non-NULL → NULL, which reads as "became canonical", while the derived value never moved.
The posting would be queued back into search while still a duplicate. The derived value is the
only one that answers the question the index asks.

### Decision 3: Both-non-NULL queues nothing, deliberately

A duplicate re-pointed at a different canon is still a duplicate, and its document is already
absent from the index. Queueing a removal would be a Meilisearch no-op — harmless, and pure
cost at ~200 documents per push. Silence here is what keeps the volume proportional to status
changes rather than to marker writes.

### Decision 4: `search_outbox` needs `job_posted_at`; take it from the returned row

`EnqueueSearchOutbox` denormalizes `COALESCE(posted_at, created_at)` so the claim can order
without joining. The passes' UPDATEs already have the row, so the CTE returns
`COALESCE(posted_at, created_at)` alongside the two markers rather than re-reading `jobs`.

### Decision 5: `:execrows` becomes `:one`

A CTE takes the affected-row count out of the command tag, so each pass's query ends in
`SELECT count(*) FROM updated` and its sqlc annotation changes from `:execrows` to `:one`.
Exactly what PR #2133 did to the five closing statements; callers keep taking `int64` and do
not change.

### Decision 6: No backfill

A posting already marked and already indexed has no transition. Backfilling would mean
enqueuing every current duplicate — on the order of a million rows — to remove documents the
next rebuild removes anyway, at a cost measured in hours of drain. The rebuild stays on its
schedule (that is Goal-adjacent, not a Non-Goal by accident), so the stock is handled and only
the flow needs this change.

## Risks / Trade-offs

- **A missed removal leaves a duplicate visible** → The transition is computed in the same
  statement as the write, so there is no window to miss one. The test that matters is the
  one-releases-while-another-holds case, which is where a plausible-looking implementation
  goes wrong.
- **Queue volume grows with dedup churn** → Bounded by status changes, not marker writes:
  ~13,385 rows a cycle across all three passes today, and only the flips among those queue.
  If a future change makes markers churn again, this queue is where it will show — which is
  better than the churn being invisible, as it was until 2026-08-19.
- **`RETURNING old./new.` ties the schema layer to Postgres 18** → Prod is 18.4 and the test
  container is `pgvector/pgvector:pg18`. The alternative costs a race; the version floor is
  already established by the running database, not raised by this change.
- **A queued removal for a job that is also closing** → Both paths queue the same job on
  `search_delete_outbox`; the queue's `ON CONFLICT` keeps one entry, and deleting an absent
  document is a Meilisearch no-op. No interaction to manage.

## Migration Plan

None. No schema change, no backfill, no worker. The change is live for every status change
after deploy, and the rebuild covers everything before it — so there is no window where the
index is worse off than it is today, and no ordering constraint between migration and code.

**Rollback** is a code revert: the queues return to being fed by ingest and the closing
statements alone, and duplicates go back to waiting for the rebuild.

**Acceptance:** after deploy, a posting marked duplicate by a refresh should disappear from
`/api/v1/jobs/search` within a drain cycle rather than at the next rebuild. The drain's own
log line reports how many removals it took, so the count appearing at all is the first signal
the producer is wired.

## Open Questions

- Does the drain's removal batch size want tuning once duplicates share the queue with
  closures? Closures alone measured ~19,827 a day; this adds status flips on top. The batch
  is 200 and a removal push measured ~7.7s, so a cycle's worth is minutes — but the number is
  worth a look after a week rather than guessed at now.
