## ADDED Requirements

### Requirement: Daily job-activity rollup

The system SHALL maintain a materialized daily rollup of catalogue flow in a
`job_daily_stats` table, keyed by calendar day (UTC), holding for each day the
count of jobs `added` (jobs whose `created_at` falls on that day) and `removed`
(jobs whose current `closed_at` falls on that day). A run-once worker SHALL
recompute the rollup from `jobs` in full and upsert every day's row, so that the
rollup is a pure function of the current `jobs` state and re-running it is
idempotent.

Because `removed` is derived from the *current* `closed_at`, a job that is later
reopened (its `closed_at` cleared) SHALL, on the next recompute, no longer be
counted as removed on its former close day.

#### Scenario: A newly created job increments its added day

- **WHEN** a job with `created_at` on day D exists and the rollup worker runs
- **THEN** `job_daily_stats` for day D has `added` including that job

#### Scenario: A closed job increments its removed day

- **WHEN** a job with `closed_at` on day D exists and the rollup worker runs
- **THEN** `job_daily_stats` for day D has `removed` including that job

#### Scenario: A reopened job drops out of its old removed day

- **WHEN** a job previously closed on day D is reopened (`closed_at` cleared) and
  the rollup worker runs again
- **THEN** `job_daily_stats` for day D no longer counts that job under `removed`

#### Scenario: Re-running the worker is idempotent

- **WHEN** the rollup worker runs twice with no change to `jobs` in between
- **THEN** the resulting `job_daily_stats` rows are identical after both runs

### Requirement: Public job-activity read endpoint

The system SHALL expose a public, unauthenticated
`GET /api/v1/stats/jobs-activity` endpoint that serves the rollup aggregated to a
requested granularity over a date range. It SHALL accept `granularity`
(`day` | `week` | `month`, default `day`) and optional `from`/`to` date bounds,
and aggregate the daily rows to the chosen period using `date_trunc`.

The response SHALL use the list envelope `{"data": [...], "meta": {...}}` where
each `data` element is `{ "period": <ISO date>, "added": <int>, "removed":
<int> }` ordered by `period` ascending, and `meta` echoes the resolved
`granularity`, `from`, and `to`.

#### Scenario: Daily granularity

- **WHEN** a client requests `GET /api/v1/stats/jobs-activity?granularity=day`
- **THEN** the response is `200` with one `data` element per day in range, each
  carrying that day's `added` and `removed`

#### Scenario: Weekly aggregation sums the constituent days

- **WHEN** a client requests `granularity=week` over a range spanning several
  weeks
- **THEN** each `data` element's `added`/`removed` equals the sum of the daily
  rollup rows within that week

#### Scenario: Invalid granularity is rejected

- **WHEN** a client requests `granularity=hour`
- **THEN** the response is `400` with an error envelope and no data is returned

#### Scenario: No authentication required

- **WHEN** an unauthenticated client requests the endpoint
- **THEN** the response is `200` (access is not gated by a session or API key)

### Requirement: Public job-activity dashboard page

The web SPA SHALL provide a public `/trends` page that renders the job-activity
data as a grouped bar chart — a green bar for `added` and a red bar for
`removed` per period — with a control to switch the granularity between day,
week, and month. The page SHALL be reachable without signing in.

#### Scenario: Chart renders added and removed bars

- **WHEN** a visitor opens `/trends`
- **THEN** the page fetches `GET /api/v1/stats/jobs-activity` and renders, per
  period, a green added bar and a red removed bar

#### Scenario: Granularity toggle re-aggregates

- **WHEN** the visitor switches the granularity control from day to month
- **THEN** the chart re-fetches at `granularity=month` and redraws with
  month-level bars
