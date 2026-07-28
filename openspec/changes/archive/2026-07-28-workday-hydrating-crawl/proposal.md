## Why

A large Workday board never finishes a crawl, and its whole catalogue leaks as permanently-open
jobs.

`dollartree.wd5.myworkdayjobs.com/dollartreeus` carries 23,919 postings. The adapter lists them
20 per page (Workday silently ignores a larger `limit` — measured: `limit:50` and `limit:100`
both return zero postings) and then fetches **every posting's detail on every hourly run** to
assemble its description. Measured on the live board: 30 consecutive listing POSTs all return
200, so the listing is not what throttles — the ~24k detail requests are. Workday answers them
with `429`, the crawl errors after 3 attempts, and the board records `last_success_at = NULL`.

The stale sweep cannot reach what a failed crawl never wrote: it scopes closes to the company
slugs a run actually recorded (`cmd/ingest/crawled.go`), and a board that ingests nothing puts
no slug in that set. So Dollar Tree's **22,756 open jobs** all sit at `last_seen_at = 2026-07-02`
— 26 days — including postings the employer has taken down (verified: the CxS detail endpoint
returns 403 for a removed requisition and 200 for a live one). Board sources have no liveness
backstop, since the probe skips registered providers. Prod-wide this shape accounts for
**327,594 open jobs unseen for over 10 days across 27,478 companies**, and 1,312 of 80,102 boards
have never recorded a successful crawl.

## What Changes

- **`workday` becomes a `HydratingSource`.** `FetchNew` lists the board as before but issues a
  detail request only for a posting the catalogue lacks; an already-ingested posting is emitted
  from the listing alone, marked `SeenRefresh`, so the pipeline refreshes its liveness without
  rewriting content. A steady-state Dollar Tree crawl drops from ~24,000 detail requests to
  roughly zero, leaving the 1,196 listing POSTs the board genuinely needs.
- **The seen-set is scoped to the board, not the provider.** Without this the capability is
  unusable for a multi-board provider: `fetchBoard` loads the seen-set **per board**, and for
  `workday` the provider-wide query returns 1,274,331 rows in **168 seconds** (140 MB of ids) —
  once per board, 6,165 boards. Scoped by the `"<board>:"` prefix of `external_id` the same
  lookup returns 25,607 rows in **1.8 seconds**, served by the existing
  `jobs_source_extid_pattern_idx (source, external_id text_pattern_ops)`. No migration.
  Prefix matching MUST use `LIKE 'prefix%'` against that index: a range predicate
  (`>= 'board:' AND < 'board;'`) returns zero rows under the database's non-C collation, where
  punctuation carries only a secondary weight.
- **A liveness refresh passes the catalogue-fit filter, judged on the stored row.** The
  `SeenRefresh` branch currently bypasses `outOfCatalogue`. Catalogue pruning depends on the
  opposite: once ingest starts rejecting a board's non-technical postings, the stored ones stop
  being seen and the sweep closes them (`docs/agents/job-lifecycle.md`). Left as is, hydration
  would `touch` Dollar Tree's 25,147 non-tech rows every hour and make them immortal. The filter
  must read the row's own `is_tech`, not what the listing implies: `ConfirmedNonTech` consults the
  dictionary only after the tech check, and measured against prod, **1,601 of 96,218 (1.7%) of the
  titles the catalogue holds as technical are flagged when judged without their description** —
  real roles like `Associate Hardware Mechanical Engineer`. Over 1.27M Workday rows that is ~21k
  live jobs a title-only rule would wrongly close. The seen-set is read from those same rows, so
  it carries `is_tech` alongside each id. This closes the same hole for the adapters that already
  hydrate (getro, hhru, infojobs, jobylon, justjoin, reed).
- **Out of scope:** re-fetching detail for a posting whose description may have changed since it
  was first hydrated. Hydration freezes stored descriptions, the accepted trade-off in every
  hydrating adapter today. Also out of scope: the recovery probe clearing the failure counter of
  a board that has never succeeded, and sharding a single oversized board.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `source-ingest`: the hydration requirement is strengthened on two points — the seen-set is
  scoped to the crawled board rather than the whole provider, and a liveness refresh is subject
  to the catalogue-fit filter so a rejected posting is not kept alive by a refresh.

## Impact

- **Code:** `internal/sources/workday.go` (`FetchNew`); `internal/pipeline/pipeline.go`
  (`seenLookup` gains the board argument, `fetchBoard` passes it, the `SeenRefresh` branch runs
  the filter); `cmd/ingest/store.go` (`ExistingExternalIDs` board scoping);
  `internal/db/queries/jobs.sql` + `make sqlc` (the prefix-scoped query).
- **Data:** no schema change and no migration — the required index already exists. Once the
  Dollar Tree board crawls again its slug re-enters the sweep scope, and every posting the
  employer removed closes on the 48h cutoff. Non-tech rows stop being refreshed and close on the
  same cutoff, which returns them to `cmd/prune`'s reach.
- **Risk:** the board-scoped seen-set is empty for a board whose stored `external_id`s do not
  carry its prefix, which would hydrate every posting — the same cost as today, so the failure
  mode is the current behaviour, not a regression. Filtering refreshes closes rows that the
  dictionary flags; that is the intended pruning path, and a dictionary term withdrawn later
  re-admits the postings on the next crawl.
