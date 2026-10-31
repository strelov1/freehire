## Context

Salary bands come from `RebuildInsightsSalaryStatsGlobal/ByCountry`, which read
`enrichment->>'salary_*'` for OPEN postings. `SetJobEnrichment` overlays manual >
source > model into that JSON, but enrichment runs only for `is_tech` postings, so a
non-technical posting's structured salary is invisible to every insights figure.

Measured on prod 2026-10-31 (canonical, public, categorized postings, by publication
month): salary-bearing postings were 2k–12k a month before May 2026, then 38k (May),
151k (June), 89k–120k (July–September), 436k (October, 398k of them from a source
field). USD per year: 260–4,200 a month before May, 18k–84k after.

## Goals / Non-Goals

**Goals:** one effective salary for every rollup; a 12-month history per category that
backfills itself; a chart that cannot be read as more precise than its sample.

**Non-Goals:**
- Per-seniority or per-country history (the sample splits too thin; revisit once
  months are large).
- Currency conversion or period normalization (the bands never do either; a converted
  median mixes exchange-rate movement into salary movement).
- Correcting for catalogue composition. A month in which freehire onboarded a source
  with a different pay mix moves the median for reasons that are not the market's. The
  page says what the figures are; see Risks.

## Decisions

### 1. The effective salary is a view, `jobs_effective_salary`

`(job_id, currency, period, amount)` plus the posting columns the salary rollups
filter and group on (category, seniority, countries, dates, duplicate_of, is_private),
so a rollup scans the view alone instead of joining it back to `jobs` — a second full
read of the table. Amount is the min/max midpoint (or whichever
bound exists) of the FIRST tier that has a bound — manual, then source, then
enrichment — with currency (upper-cased) and period from that same tier. Rows without
a usable figure are absent. Both band rebuilds and the monthly rebuild join it.

*Alternative:* repeat a `CASE` in each statement. Three copies of a three-tier rule is
exactly how the bands came to read one tier only; sqlc cannot share a SQL fragment,
and a view is the database's own way to name an expression once. Postgres inlines a
simple view, so it costs nothing at plan time.

*Alternative:* overlay the source salary into `enrichment` at ingest for every
posting. It would fix the rollups, but writes to millions of rows and changes what
"enrichment" means for postings the model never saw.

### 2. History is a pure function of `jobs`, not a snapshot

Postings are never hard-deleted, so the offers of any past month are still on the
rows; grouping by publication month recomputes all 12 months each run. That is the
backfill. A snapshot table (like `insights_skill_history`) would start empty and
could never recover the months before it shipped.

Window: `LEAST(posted_at, created_at) >= date_trunc('month', now()) - 11 months`.
Open AND closed postings: a posting closed since still records an offer of its month.

### 3. Floor of 30 per point, stored, not presented

The rollup stores only months with ≥ 30 postings (the bands use 5; a trend line
drawn through five points per month is noise presented as a trend). The API returns
what is stored; the page needs ≥ 3 points to draw anything.

### 4. Chart: the existing hand-drawn SVG pattern

`web/src/lib/skillDetailChart.ts` + `activityChart.pickTickIndices` already draw a
time series as SVG with no chart library; the salary chart follows them: a pure model
(`salaryHistoryChart.ts`: x per month slot, y scaled to the series' p25..p75 range,
segments split at gaps) with vitest tests, and a thin component. The current month is
partial; its point is drawn hollow and labelled "so far". A visually hidden list of
buttons carries the same numbers for screen readers and no-JS readers and is the
keyboard's way along the line, as on the skill-pulse chart. In-SVG axis text shrinks to
~4px on a phone, so there it is hidden and the range is written as text below.

## Risks / Trade-offs

- [Composition shifts move the median] → a month whose sample is ≥ 3× the month
  before is drawn with a dashed step and says so in its tooltip (backend EUR/year:
  €62,500 → €45,000 as the sample went 104 → 548). The sample size is in every tooltip.
- [Bands change on deploy] → expected: they gain non-technical source salaries.
  Called out in the PR with before/after for one non-technical category.
- [Rollup run time] → one scan of ~12 months of postings with a jsonb read; measured
  ~5–8 min for the equivalent count on prod. Checked with EXPLAIN before merge.
- [Early months are biased toward long-lived postings] → they are found late and
  survive; the 30 floor keeps most of them off the chart, and the month is labelled
  by publication date.

## Migration Plan

One migration: `CREATE OR REPLACE VIEW jobs_effective_salary` and
`CREATE TABLE IF NOT EXISTS insights_salary_monthly`. `release.sh` migrates before
the flip. The table is empty until the next rollup run; the page draws no chart from
an empty history. Rollback: revert the binary; the view and table are inert.
