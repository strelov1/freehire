## Why

The ingest-time aggregator coverage gate asks one question — "does this `company_slug` have any
OPEN posting from a non-aggregator source?" — and on a yes it does not save the aggregator's
posting. The answer is a claim about the present tense ("we already crawl this employer"), but
it is read off a row that may have been last seen weeks ago from a board that is no longer in
`sources/`. Nothing in the gate requires the coverage to be current.

Issue #2315 reports the consequence twice. `https://himalayas.app/companies/pipe/jobs/senior-software-engineer-5097528323`
is a live remote US software job that `GET /api/v1/jobs/find` answers `{"data":null}` for. The
slug `pipe` is held "covered" by this row:

```text
source: trakstar   posted_at: 2013-11-07   last_seen_at: 2026-08-02   (31 days unseen)
title: Business Development, Houston TX — a carbon steel line pipe distributor
board Pipe.hire.trakstar.com is absent from sources/trakstar.yml
```

A different employer, a non-technical posting from 2013, from a board we no longer crawl. It
will never be seen again, so it will hold the slug forever, and every future Himalayas posting
for the fintech Pipe is discarded unsaved. The same row also carries the `unicorn` collection
tag the fintech earned — the identity collision is already visible on the public company page.

Measured on prod 2026-09-02 over every company slug the gate currently treats as covered:

| Newest non-aggregator row for the slug | Slugs | Share |
|---|---|---|
| **Total slugs with non-aggregator coverage** | **203,441** | 100% |
| unseen > 48h | 43,649 | 21.5% |
| unseen > 7 days | 31,001 | 15.2% |
| unseen > 14 days | 22,022 | 10.8% |
| unseen > 30 days | 11,151 | 5.5% |

Between 11k and 22k employers are closed to aggregator ingest by rows nobody has seen in weeks.

## What Changes

- **Coverage must be CURRENT.** A non-aggregator posting counts as coverage only when its
  `last_seen_at` is within a freshness window. Outside it, the company reads as uncovered and
  the aggregator's posting is saved as normal.
- **The window is 14 days**, chosen against a measurement of real crawl cadence rather than
  from the sweep's 48h grace — see design.md. It is a package constant, not an env var.
- **`CoverageLookup` moves from Meilisearch to Postgres.** `last_seen_at` structurally cannot
  reach the search index (see design.md), so the lookup has to read the table that holds it.
- **Hyphen-folding stops being a workaround and becomes the query.** Postgres has the stored
  `jobs.company_slug_folded` column and its partial index (migration 0109 / 0076), so the fold
  is one indexed predicate instead of the two-clause OR the Meili filter language forced.
- **`internal/search/search/coverage.go` is deleted**, along with `JobDocument.CompanySlugFolded`
  and the `company_slug_folded` entry in the jobs index's `FilterableAttributes` — that field
  existed for this gate alone and has no other reader.

## Impact

- Affected specs: `aggregator-ats-coverage-skip`
- Affected code: `internal/ingest/pipeline` (the port's doc contract), a new Postgres-backed
  implementation and its query, `cmd/ingest` (wiring), `internal/search/search` (deletions).
- **Behavioural:** more aggregator postings are saved. The gate's own counter
  (`Stats.ATSCovered`) falls; catalogue growth is bounded by the 22,022 slugs above, and every
  posting newly admitted is one the `aggregator-ats-dedup` reindex pass can still mark later if
  the employer really is the same. Erring this way is the recoverable direction — see design.md.
- **Migration:** one, `0122`, recording `jobs_open_company_slug_folded_col_idx`. The index has
  been on prod since 0109, but only because that file's COMMENT told an operator to build it by
  hand — the file itself never created it, so any volume built from `migrations/` has the column
  and no index. `IF NOT EXISTS` makes it a no-op on prod.
- **Not in scope:** the reason those rows stay open at all. A board that leaves `sources/`
  takes its company slugs out of the ingest sweep's scope forever, so its rows are never
  closed (`docs/agents/job-lifecycle.md` documents this as a deliberate under-close). Filed as
  **freehire#2328**; this change stops those rows from suppressing live postings, and does not
  close them.
