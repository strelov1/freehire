## Context

The spike's first attempt — the same grouped scan, ad hoc, under a 180s statement
timeout — did not finish on a host at load 37. `cmd/rollup-stats` already runs exactly
that scan every 3 hours with no such bound and finishes (snapshot published 10:56, 13:47,
18:28 on 2026-10-29). So the counts ride THAT scan: `FILTER` clauses on an aggregate
already reading every open row cost CPU per row, not I/O.

## Decisions

**Ride the existing scan, not a new worker.** A second full pass over the open catalogue
would be the cost the spike measured. Rejected: Soda/Great Expectations — a second
toolchain that would run the same SQL.

**Counts, not ratios.** The repository's rule for these families (see
`freehire_ingest_rows_written`): which share deserves a page depends on the source, so the
division and the threshold belong to the alert rule. Several sources are legitimately
geo-less or never re-listed (telegram), so the rule alerts on a CHANGE in a source's share
over a week, gated on a minimum size.

**No description read.** "Has no description" would be the most direct completeness check
but a `description` predicate de-TOASTs every row — the reason the scan's own comment
forbids it. Omitted; the five checks read narrow columns only.

**Separate metrics query.** `ListSourceStats` feeds the public `/api/v1/sources`; the
gauges read their own query so the public read does not change shape.

**The checks' definitions:**

| check | open posting counted when |
|---|---|
| `missing_work_mode` | `work_mode = ''` (the column is NOT NULL; unknown is empty) |
| `missing_geo` | no `countries` and no `regions` |
| `missing_company` | `company_slug = ''` (NOT NULL; unknown is empty) |
| `unseen_7d` | `last_seen_at` older than 7 days |
| `posted_over_60d` | `COALESCE(posted_at, created_at)` older than 60 days |

## Risks

- A source whose crawl never refreshes `last_seen_at` (telegram, manual) reads ~100%
  `unseen_7d` permanently — a level, not a jump, so the change-based rule does not fire.
- The snapshot is 3 hours stale by design; the alerts are for regressions over days.
