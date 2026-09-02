# Design

## Why the company scope leaks, stated precisely

The sweep asks "did this run cover the thing I am about to close?" and answers it with the set
of company slugs the run wrote. That answer is derived from postings, so a company with no
posting this run is indistinguishable from a company on a board the run never reached. The
sweep must therefore assume the second, and it leaks the first.

The board is the unit that answers the question directly. It is also the unit the fleet is
sharded by (`--shard=i/n` splits a board FILE, never a board), so a board is either crawled in
full or not touched at all — exactly the property the company slug lacks.

## What proves a board was covered

Three conditions, all already computed:

1. **The crawl did not fail.** `pipeline.recordSuccess` is the existing decision point.
2. **The board yielded at least one posting.** Not "saved at least one" — `Ingested + Rejected +
   ATSCovered > 0`, which is `boardReachedPostings`, the predicate `pipeline.go` already used to
   decide a mid-crawl error was still progress. A board whose postings the catalogue filter all
   rejected was still listed; using the saved count instead would refuse to sweep exactly the
   non-tech-heavy boards where old rows accumulate. `Skipped` is deliberately NOT counted: it
   means the posting was listed and then failed to PERSIST, so counting it would let a board
   whose every save is failing prove itself on the strength of those failures.
3. **The entry names a board.** A boardless entry (`board == ""`) namespaces its postings as
   `":<id>"`, so `BoardPattern("")` would match the provider's whole catalogue. There is no
   board scope to speak of; the company scope keeps those.

Condition 2 is the one that carries the safety, and it is not an optimisation. A board that
returns ZERO postings is indistinguishable from a board whose crawl broke — which is not
hypothetical: a Workday board once returned `total:0` on its second page, the crawl stopped at
40 of 648 postings, and the sweep closed the live tail (freehire#725). Measured on prod, the
zero-yield boards account for 19,693 of the 235,313 candidate rows: refusing them costs 8.4% of
the benefit and removes the entire class of failure.

## Which providers are excluded, and on what

Keyed on markers, never on a list of names:

- **`sweepGrace`** — the marker's own doc says the crawl "deliberately reaches only a SLICE of
  the source's catalogue". That is the definition of a board this rule must not close within.
  Today: `adzuna`, `echojobs`, `seek`. All aggregators, none board-based, so the exclusion costs
  nothing today — but it is the structural guard, not an observation about today's registry.
- **self-closing** (`jobtech`) — its feed's removals are authoritative; already excluded.
- **`fullCatalog`** (`habr_career`, `geekjob`) — already closes by source alone on a clean run,
  which is strictly broader than a board scope.

## How the qualifying set reaches the sweep

`Runner.Run` already returns a per-provider `Stats`, which is the run's report. The qualifying
boards go there.

The alternative was widening the `BoardHealth` port, which the Runner already calls once per
board with that board's outcome. It was rejected: that port is about a board's HEALTH — failure
counts, cooldowns, freshness — and the sweep's scope is not a health question. Threading scope
through it would make one port answer two unrelated questions, and `cmd/ingest` would then read
back through a database round-trip a fact the run had in memory a moment earlier.

## The query

`CloseUnseenJobsForBoard` is `CloseUnseenJobs` with the `company_slug = ANY(...)` predicate
swapped for `external_id LIKE`. Everything else about it must be copied exactly, and one part
of that is easy to drop:

```text
WITH closed AS (
    UPDATE jobs
    SET closed_at = now(), closed_reason = 'unseen', updated_at = now()
    WHERE closed_at IS NULL
      AND source = @source
      AND last_seen_at < @cutoff
      AND external_id LIKE @board_pattern   -- externalid.BoardPattern(board)
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;
```

**The `search_delete_outbox` enqueue is not optional.** It rides the same statement in
`CloseUnseenJobs` so the enqueue is atomic with the close (a rolled-back sweep queues nothing)
and exact (only rows that actually closed are queued). A board-scoped close that omitted it
would close 215,000 rows in Postgres and leave every one of them in the search index until the
next full rebuild.

`BoardPattern` already exists and already escapes the board's LIKE metacharacters; it is what
the seen-set uses, and `internal/platform/db`'s integration test already asserts it agrees with
`Namespace`. The predicate rides `(source, external_id text_pattern_ops)`. No migration.

`closed_reason` stays `'unseen'` rather than gaining a new value: this is the same mechanism —
"a crawl of the place this posting lived did not list it" — reaching rows its scope previously
could not. A new reason would imply a fifth mechanism operators must reason about separately,
and `job-lifecycle` deliberately keeps the reason set to one value per MECHANISM.

**No row-by-row fallback is added.** `CloseUnseenJobs` has one (`UnseenJobIDs` +
`CloseUnseenJobByID`) because a single corrupted row aborts the bulk UPDATE and, at provider
scope, that blocks every closeable row of the provider — the 2026-08-11 incident, where one
duplicated `jobs_pkey` value blocked greenhouse's sweep on every run. At BOARD scope the blast
radius of the same corruption is one board, and the other boards' statements are unaffected. A
per-board failure is logged and the sweep continues to the next board; duplicating the fallback
machinery to rescue one board is not worth the second code path.

## Why not a fifth mechanism reading `board_health`

The measurement that motivated this change was taken by joining stale jobs to
`board_health.last_success_at`, and the obvious implementation is to keep doing that in a new
worker. It was rejected:

- It compensates for the leak instead of removing it. The sweep would keep under-closing and a
  second pass would keep cleaning up after it.
- It needs a cross-run join to recover a fact the run itself held: the board just ran, and this
  is what it listed.
- `board_health.last_ingested_count` is the LAST run's count, so the join's "did it yield"
  test is only ever approximately the run that mattered.

The board-scoped sweep reaches the same rows, one board at a time, as each board comes round.
The backlog drains over one fleet cycle instead of one pass — which is the only thing the
rejected design does better, and it does not justify a second mechanism in a lifecycle that
already documents four.

## What the enclosing loop already decides

`sweepableProviders` gates the whole per-provider sweep on `Ingested > 0`, and that is not
changed here. A consequence worth stating rather than discovering: a provider whose every
posting this run was rejected by the catalogue filter never reaches EITHER close, even though
the pipeline correctly marked its boards sweepable — a rejected posting is one the crawl
reached, which is why it counts toward `boardReachedPostings`.

The two rules answer different questions and both are right. The board qualification asks "did
we read this board", the provider gate asks "did this run see enough of the world to justify
closing anything". A board caught by the gap simply waits for the next run in which its provider
saves something. Widening the gate would mean deciding that a provider whose entire crawl was
filtered away is nevertheless known-good, which is a larger claim than this change needs.

## Rollout

Live from the first run, with a per-board log line carrying the board and the count. The
existing provider-level sweep log stays; the per-board line is what makes the first cycle
readable, since a provider-level number cannot distinguish "many boards each retiring a few
rows" from "one board mass-closing".

Reversibility is the existing playbook, unchanged: closing is soft, and re-ingesting a single
board reopens everything it re-lists, because `UpsertJob`'s `ON CONFLICT` clears `closed_at`.
A temporary one-board `sources/<provider>.yml` is the documented remediation.

## Testing

The interesting cases are all about what must NOT be closed, and each is a unit test against
the pipeline/sweep seam with a fake store:

- a board that yielded zero postings closes nothing, even though its crawl succeeded;
- a board whose crawl failed closes nothing;
- a boardless entry closes nothing through the board scope;
- a provider declaring `sweepGrace` closes nothing through the board scope;
- a company the run wrote NO posting for, on a board that yielded, IS closed — the leak;
- one board's close never touches another board of the same provider.

The SQL half — that `BoardPattern` selects one board's rows and not a prefix-sharing
neighbour's — is an integration test in `internal/platform/db`, beside the existing
`BoardTracked` one that pins the same pattern.
