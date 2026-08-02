## Why

Opening `/my/tracking` server-loads 500 rows in one call, and each row carries the full text of
its job posting — which the board renders nowhere. Measured against a seeded 500-application
board with 5 KB descriptions (`TestMeasureBoardLoad`):

```
payload:               2.83 MB
of which descriptions: 2.37 MB  (84%)
scan WITH description:    18.8 ms
scan WITHOUT description:  6.0 ms
full query (5 correlated subqueries): 39.7 ms
endpoint:                             52.6 ms
```

So the description costs 84% of the bytes and roughly 13 ms of the 40. Those bytes do not merely
travel: the tracking routes server-load the board, so they are serialized into the SSR payload,
embedded in the HTML document, and parsed again by the browser.

The board needs an employer, a role, and a handful of facets for its tag row. It is asking for
the whole posting.

## What Changes

- **BREAKING**: `GET /me/tracking` returns a **card** projection of the job — `public_slug`,
  `title`, `company`, `closed_at`, and the flat facets the tag row reads (`work_mode`,
  `seniority`, `employment_type`, `countries`, `regions`) — instead of the full `jobview.Job`.
  The full posting stays available, unchanged, at `GET /me/tracking/:slug`, which the drawer
  already fetches for its linked mail.
- A dedicated listing query reads only those columns. `sqlc.embed(jobs)` is what pulled all 51,
  and dropping the description from the scan is what removes the 13 ms — obliterating it in Go
  would save the bytes and none of the read.
- The three correlated subqueries over `emails` (count, newest received, pending suggestion)
  collapse into one `LATERAL` — one pass over the same rows, and one spelling of a predicate
  currently written three times.
- **No indexes.** They were in this proposal until they were measured. Against a fixture with a
  realistic mailbox — 5 000 messages of which 167 are linked — the listing ran 2.62 ms with them
  and 2.79 ms without: noise. What did move the number by two orders of magnitude was `ANALYZE`
  after the bulk insert (216 ms → 2.7 ms), which is a fixture artefact; production statistics
  are maintained by autovacuum. Adding an index that pays for itself on every write, to buy
  nothing measurable on the read, is the infrastructure-before-need this repo warns against.
- The drawer reads the description and the full facet row from the detail response it already
  loads, rather than from the listing.

Deliberately NOT done: no materialized view, no denormalized card table. Both add a second
source of truth and a synchronization path to keep honest, and the measurement says the cost is
bytes we choose to send, not work the database cannot avoid.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `user-job-tracking`: the listing returns a card projection of the job rather than the full
  public job view; the full view remains on the single-application read.

## Impact

- **Go**: `internal/db/queries/user_jobs.sql` (card query + LATERAL), `internal/handler/me_tracking.go`,
  a card wire type, `internal/jobtracking`.
- **SQL**: none. No migration, no column or table changes — the listing query is rewritten in
  place and the indexes were measured and dropped from scope.
- **Frontend**: `web/src/lib/types.ts`, `board.ts`, `JobDrawer.svelte` (description and tags from
  the detail response), `BoardCard.svelte`, `BoardList.svelte`.
- **Docs**: `docs/API.md`, `web/src/lib/docs/api-spec.ts`, `internal/userjob/AGENTS.md`.
- **Tests**: `TestMeasureBoardLoad` becomes a regression guard — the listing must not carry a
  description, and the payload must stay under a stated ceiling.

## Result

Measured the same way, after the change:

```
payload:  2.83 MB → 0.23 MB   (-92%)
scan with description 6.5 ms → without 0.8 ms
query:    2.79 ms      endpoint: 4.61 ms
```
