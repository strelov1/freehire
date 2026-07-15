# market-insights Specification

## Purpose

Expose the job catalogue's aggregate market intelligence — role demand, skill
demand, hiring velocity, and salary bands — as public, unauthenticated,
aggregate-only read APIs served from precomputed rollups. This turns the
structured enrichment facets already stored per job into answers to catalogue-wide
questions ("which roles are hiring", "what skills are in demand", "what does this
role pay") without exposing any record-level field.

## Requirements

### Requirement: Role demand insights

The system SHALL expose a public, unauthenticated `GET /api/v1/insights/roles`
endpoint that returns roles (identified by category × seniority) ranked by the
number of currently open jobs, together with a growth measure comparing the
current open-count to the open-count a fixed window earlier. The endpoint SHALL
accept optional geography scoping (country or region) and a result limit, and
SHALL respond with the standard list envelope `{"data": [...], "meta": {...}}`.

#### Scenario: Top roles by open count

- **WHEN** a client requests `GET /api/v1/insights/roles` with no filters
- **THEN** the response is `200` with a `data` array of roles, each carrying
  `category`, `seniority`, `open_count`, and a `growth` measure, ordered by
  `open_count` descending

#### Scenario: Roles scoped by geography

- **WHEN** a client requests `GET /api/v1/insights/roles?country=DE`
- **THEN** only jobs whose countries include `DE` contribute to the counts, and
  `meta` echoes the applied `country` filter

#### Scenario: Fastest-growing roles

- **WHEN** a client requests `GET /api/v1/insights/roles?sort=growth`
- **THEN** roles are ordered by their growth measure descending rather than by
  raw open-count

#### Scenario: Invalid parameter rejected

- **WHEN** a client requests `GET /api/v1/insights/roles?sort=bogus`
- **THEN** the response is `400` with an `{"error": ...}` body and no partial data

### Requirement: Skill demand insights

The system SHALL expose a public, unauthenticated `GET /api/v1/insights/skills`
endpoint that returns skills ranked by the number of currently open jobs that
list them, together with a growth measure over a fixed window. The endpoint SHALL
accept optional geography and category scoping and a result limit, and SHALL
respond with the standard list envelope.

#### Scenario: Top skills by demand

- **WHEN** a client requests `GET /api/v1/insights/skills`
- **THEN** the response is `200` with a `data` array of skills, each carrying the
  canonical `skill`, `open_count`, and `growth`, ordered by `open_count`
  descending

#### Scenario: Skills scoped by category

- **WHEN** a client requests `GET /api/v1/insights/skills?category=engineering`
- **THEN** only jobs in that category contribute to the skill counts

### Requirement: Hiring velocity insights

The system SHALL expose a public, unauthenticated `GET /api/v1/insights/velocity`
endpoint that returns a dense, gap-free time series of jobs added versus removed
over a validated date range and granularity (day/week/month), optionally scoped
to a single facet value (e.g. a category, seniority, or country). Missing periods
SHALL appear as zeros. The endpoint SHALL respond with the standard list envelope
and echo the resolved window in `meta`.

#### Scenario: Global velocity series

- **WHEN** a client requests `GET /api/v1/insights/velocity?granularity=week`
- **THEN** the response is `200` with a `data` array of `{period, added, removed}`
  points at weekly granularity, and `meta` carries the resolved `granularity`,
  `from`, and `to`

#### Scenario: Velocity scoped to a facet

- **WHEN** a client requests `GET /api/v1/insights/velocity?category=engineering`
- **THEN** the added/removed counts reflect only jobs in that category

#### Scenario: Range too large rejected

- **WHEN** a client requests a `from`/`to` span exceeding the configured maximum
- **THEN** the response is `400` with an `{"error": ...}` body

### Requirement: Salary band insights

The system SHALL expose a public, unauthenticated `GET /api/v1/insights/salary`
endpoint that returns salary distribution bands (at minimum the 25th, 50th, and
75th percentiles) for a role, reported separately per currency and normalized pay
period, computed only from jobs that disclose a single, comparable salary figure.
The endpoint SHALL accept role scoping (category and/or seniority) and geography
scoping, and SHALL respond with the standard envelope.

#### Scenario: Salary bands for a role

- **WHEN** a client requests `GET /api/v1/insights/salary?category=engineering&seniority=senior`
- **THEN** the response is `200` with `data` entries each carrying `currency`,
  `period`, `p25`, `p50`, `p75`, and the contributing `sample_size`

#### Scenario: Currencies never mixed

- **WHEN** jobs in the requested scope disclose salaries in multiple currencies
- **THEN** each currency yields its own band entry and figures from different
  currencies are never combined into one percentile

#### Scenario: Small samples suppressed

- **WHEN** a currency/period band for the requested scope has fewer contributing
  jobs than the configured minimum sample size
- **THEN** that band is omitted rather than returned with an unreliable or
  potentially identifying figure

### Requirement: Precomputed insights rollups

Insight reads SHALL be served from precomputed rollup tables rather than
aggregating the full `jobs` table on each request. The rollups SHALL be a pure
function of current `jobs` state — open-as-of-a-date derived from `created_at`
and `closed_at` — recomputed by a cron-scheduled run-once worker and swapped in
atomically so readers never observe a partially rebuilt rollup.

#### Scenario: Reader never sees a partial rebuild

- **WHEN** the rollup worker is mid-recompute
- **THEN** concurrent insight reads return the previous complete snapshot until
  the recompute commits

#### Scenario: Recompute is idempotent

- **WHEN** the rollup worker runs twice with unchanged `jobs` state
- **THEN** the resulting rollup tables are identical

### Requirement: Aggregate-only, abuse-safe reads

Every insights endpoint SHALL be aggregate-only: no per-job, per-user, or
per-company identifier or free-text field SHALL appear in any response. All query
parameters SHALL be validated against whitelists (enumerations, bounded limits,
bounded date ranges) before use, and no parameter value SHALL be interpolated
into SQL.

#### Scenario: No record-level data leaks

- **WHEN** any insights endpoint responds successfully
- **THEN** the payload contains only aggregate counts, percentiles, and facet
  labels — never a job slug, user id, company id, title, or description

#### Scenario: Unbounded limit rejected

- **WHEN** a client requests a result `limit` above the configured maximum
- **THEN** the response is `400` rather than an unbounded scan
