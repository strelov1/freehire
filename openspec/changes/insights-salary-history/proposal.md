## Why

`/insights/salary/<category>` shows what a role pays today and nothing about where it
is heading — the question a candidate negotiating an offer actually asks. The history
can be built from the catalogue itself: postings are never hard-deleted, so the
salaries advertised in every month freehire has seen are still on the rows.

Building it exposed a gap in today's bands. They read the salary only from
`enrichment`, which is written by LLM enrichment, which runs for technical postings
only. A structured salary an ATS states (`salary_*_source`) or a moderator sets
(`salary_*_manual`) therefore never reaches the bands for any non-technical posting.
Measured on prod 2026-10-31: of postings published in October 2026, 397,884 carry a
source salary, against 38,192 whose only salary came from the model.

## What Changes

- **One effective salary.** A database view resolves a posting's salary once —
  manual, else source, else the enrichment figure — and every salary rollup reads it.
  The existing bands change VALUES (non-technical postings with an ATS salary now
  count); their shape and endpoint do not.
- **Monthly salary history** per category and (currency, period): p25/p50/p75 and the
  sample size for each of the last 12 months, by the month a posting was PUBLISHED
  (`LEAST(posted_at, created_at)`), open and closed postings alike. Recomputed in full
  on every rollup run, so it is backfilled the moment it ships.
- **New endpoint** `GET /api/v1/insights/salary/history?category=<c>`.
- **A chart on `/insights/salary/<category>`**: the median line and the p25–p75 band by
  month, for the category's most common currency and period, with a switch for the
  others.

## Capabilities

### New Capabilities

- `insights-salary-history`: the monthly salary rollup, its endpoint and the chart.

### Modified Capabilities

- `market-insights`: "Salary band insights" — bands read the effective salary
  (manual > source > enrichment) rather than the enrichment figure alone.

## Impact

- New migration: the `jobs_effective_salary` view and the
  `insights_salary_monthly` table.
- `internal/platform/db/queries/insights.sql`: both band rebuilds read the view; a new
  monthly rebuild and read query. `cmd/rollup-stats`: one more rebuild.
- `internal/api/handler/insights.go` + route.
- `web/src/routes/insights/salary/[category]/`, a chart component and its pure model.
- No LLM spend, no Meilisearch change, no reindex. The insights API is not in
  `web/static/openapi.yaml`.
- The rollup gains one scan of the postings published in the last 12 months.
