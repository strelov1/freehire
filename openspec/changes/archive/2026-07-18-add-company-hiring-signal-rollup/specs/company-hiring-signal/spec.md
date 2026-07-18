## ADDED Requirements

### Requirement: Per-company daily hiring-velocity rollup

The system SHALL maintain a precomputed rollup table `insights_company_stats` keyed by `(company_slug, day)` that records, for each company and each UTC day the company had activity, the number of jobs `added` (jobs whose `created_at` falls on that day), `removed` (jobs whose current `closed_at` falls on that day), and `open` (jobs open as of the end of that day).

Openness for a day `D` SHALL be computed as `created_at <= D AND (closed_at IS NULL OR closed_at > D)`, matching the convention documented in `migrations/0022_insights_rollups.sql`.

Only canonical, attributable job rows SHALL be counted: rows with a non-empty `company_slug` and `duplicate_of IS NULL` (repost copies are excluded so counts match `companies.job_count` semantics).

#### Scenario: A company's posting is reflected as added and open

- **WHEN** a company has one job with `created_at = 2026-01-10` and `closed_at IS NULL`, and the rollup is rebuilt
- **THEN** `insights_company_stats` has a row for that `company_slug` on day `2026-01-10` with `added = 1`
- **AND** `open` is `1` for that company on `2026-01-10` and every subsequent day up to the rollup's latest day

#### Scenario: A closed posting is reflected as removed

- **WHEN** a company's job has `created_at = 2026-01-10` and `closed_at = 2026-01-20`
- **THEN** the rollup records `added = 1` on `2026-01-10` and `removed = 1` on `2026-01-20`
- **AND** `open` for that company is `0` from `2026-01-20` onward (the close day itself is no longer open)

#### Scenario: Repost copies are not double-counted

- **WHEN** a job row has `duplicate_of` pointing at a canonical row
- **THEN** that duplicate row contributes nothing to `added`, `removed`, or `open`

#### Scenario: Jobs without a company are excluded

- **WHEN** a job row has an empty `company_slug`
- **THEN** it contributes to no `insights_company_stats` row

### Requirement: Retroactive full-history rebuild

The rollup SHALL be reconstructable in full from the current `jobs` table alone, back to the earliest `created_at`, because closed jobs are retained rather than deleted. A rebuild SHALL NOT depend on any prior rollup state or external snapshot.

#### Scenario: First rebuild reconstructs prior days

- **WHEN** the rollup is run for the first time against a catalogue containing jobs created and closed across past days
- **THEN** `insights_company_stats` contains rows for those past days derived solely from `created_at`/`closed_at`

### Requirement: Hiring growth derivable from the open time series

Hiring growth (ramping vs. freezing) over any window SHALL be derivable from the stored per-day `open` series alone, without scanning `jobs`: a company's open count as of a date is the value of `open` on its latest rollup row at or before that date (carry-forward across days with no activity). The rollup therefore SHALL NOT need a separate stored "previous window" column.

#### Scenario: 30-day growth reads from the open series

- **WHEN** a company's rollup shows `open = 4` on its latest row at or before 30 days ago and `open = 10` on its most recent row
- **THEN** its 30-day growth is `10 - 4 = 6`, computed from `insights_company_stats` alone

### Requirement: Atomic rebuild worker

A run-once-and-exit worker `cmd/rollup-company` SHALL rebuild `insights_company_stats` inside a single database transaction using a `DELETE`-then-`INSERT` recompute, so that readers never observe a partially rebuilt table. The worker SHALL require only `DATABASE_URL` and SHALL exit non-zero on failure without leaving the table partially written.

#### Scenario: Rebuild is all-or-nothing

- **WHEN** the rebuild transaction fails partway
- **THEN** the previously committed contents of `insights_company_stats` remain intact (no partial state is committed)

#### Scenario: Successful run replaces contents atomically

- **WHEN** the worker completes successfully
- **THEN** `insights_company_stats` reflects exactly the recomputed rows for the current `jobs` state
