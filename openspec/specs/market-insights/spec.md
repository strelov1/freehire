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

### Requirement: Category-scoped role demand

The `GET /api/v1/insights/roles` endpoint SHALL accept an optional `category`
parameter that restricts the ranked roles to that category's seniorities, so a
per-category roles view can be served in a single call. The parameter SHALL be
validated against the enrichment category vocabulary; an unknown value SHALL be a
`400`. When omitted, the endpoint behaves as before (all category × seniority pairs).

#### Scenario: Roles scoped to one category

- **WHEN** a client requests `GET /api/v1/insights/roles?category=backend`
- **THEN** the `data` array contains only `backend` roles (one row per seniority
  present), ranked by the requested sort, and `meta` echoes the `category`

#### Scenario: Unknown category rejected

- **WHEN** a client requests `GET /api/v1/insights/roles?category=not-a-category`
- **THEN** the response is `400` with an `{"error": ...}` body

### Requirement: All-seniority salary bands for a category

The system SHALL provide a way to read, in a single call, the salary bands for every
seniority within a category (each per currency and period), so a per-category salary
page does not need one request per seniority. Bands below the minimum sample size
SHALL remain suppressed as for the existing salary read.

#### Scenario: Category salary spans seniorities in one call

- **WHEN** a client requests the category salary read for `backend`
- **THEN** the response contains the salary bands for each seniority in `backend`
  that has a qualifying sample, grouped by seniority and currency, in one response

### Requirement: Company hiring-signal leaderboard

The system SHALL expose a public, unauthenticated `GET /api/v1/insights/companies`
endpoint that returns companies ranked by hiring growth or current open-count,
served from a precomputed per-company scalar (never aggregated per request over the
full catalogue). It SHALL accept `sort` (`growth` = ramping first, `-growth` =
freezing first, `open` = largest first; default `growth`), a `min_open` threshold
(a small positive default) that excludes companies whose current open-count is below
it, and a `limit` capped to a fixed maximum. It SHALL respond with the standard list
envelope `{"data": [...], "meta": {...}}` where each row carries `company_slug`,
`company_name`, `open_now`, `open_prev_30d`, and `growth_30d`. The response SHALL be
aggregate-only — no record-level job fields.

#### Scenario: Top ramping companies

- **WHEN** a client requests `GET /api/v1/insights/companies?sort=growth`
- **THEN** the response is `200` with a `data` array ordered by
  `growth_30d` (= `open_now − open_prev_30d`) descending, each row carrying
  `company_slug`, `company_name`, `open_now`, `open_prev_30d`, `growth_30d`
- **AND** `meta` echoes the applied `sort`, `min_open`, and `limit`

#### Scenario: Freezing companies

- **WHEN** a client requests `GET /api/v1/insights/companies?sort=-growth`
- **THEN** companies are ordered by `growth_30d` ascending (largest declines first)

#### Scenario: min_open excludes small companies

- **WHEN** a client requests `GET /api/v1/insights/companies?min_open=10`
- **THEN** only companies whose `open_now` is at least `10` appear in `data`

#### Scenario: Invalid sort rejected

- **WHEN** a client requests `GET /api/v1/insights/companies?sort=bogus`
- **THEN** the response is `400` and no data is returned

#### Scenario: Limit is capped

- **WHEN** a client requests a `limit` above the endpoint's maximum
- **THEN** the applied limit is clamped to that maximum (or `400`), never unbounded

### Requirement: Per-role skill demand

The system SHALL expose the skill distribution *within* a single role, where a role is the
pair (category, seniority). Naming a single role on the existing public, unauthenticated
`GET /api/v1/insights/roles` endpoint — by supplying both `category` and `seniority` —
SHALL return, in addition to that role's `open_count` and `growth`, the canonical skills
that role's currently-open postings carry, each with the count of those postings and that
count as a share, ordered by count descending.

The share's denominator SHALL be the number of the role's open postings that carry AT
LEAST ONE tagged skill, never the role's whole open count, and that denominator SHALL be
served beside the distribution as `sample_size`. Measured 2026-09-18 on production, 11% of
the eligible postings (392,020 → 348,060) carry no tagged skill at all; dividing by the
whole open count would deflate every share by our own tagging gap, and because that gap
differs per role it would make two roles' shares incomparable — which is the one
comparison the surface exists to support.

The role's open count and its `sample_size` are therefore different numbers, and a surface
SHALL render the share against `sample_size`, never against `open_count`.

The distribution SHALL be served from a precomputed rollup, never counted on the request
path, and SHALL be recomputed by the same worker and in the same atomic
delete-and-reinsert transaction as the sibling insights rollups. A skill whose count
within the role falls below the worker's sample floor SHALL be omitted, so a share is
never published off a sample too small to mean anything.

The distribution measures skills the postings CARRY, which is what the `jobs.skills` facet
records — a skill named anywhere in a posting, including in a "nice to have" list or a
stack description. It is therefore an upper bound on what the role requires, and every
surface rendering it SHALL say "mentioned in", never "required by".

The distribution describes the postings that STATE a seniority, not the role's market.
Measured 2026-09-18 on production, only 39.0% of open `is_tech` postings carry a
non-empty `seniority` (394,610 of 1,012,085), against 98.7% for `category` — and the
missing 61% are not a random sample, being exactly the postings whose title names no
level. Every surface rendering a single role's distribution SHALL therefore state the
`sample_size` it was measured over, and SHALL NOT claim to describe the role's market.

The rollup SHALL NOT be scoped by geography. `GET /api/v1/insights/roles` continues to
accept `country`, and when a country is supplied alongside a single role the response
SHALL carry the country-scoped `open_count` and `growth` as before, together with the
country-agnostic skill distribution, and `meta` SHALL state that the skill distribution is
not geography-scoped.

#### Scenario: Skills for one named role

- **WHEN** a client requests `GET /api/v1/insights/roles?category=backend&seniority=senior`
- **THEN** the response is `200` and `data` carries that single role with `category`,
  `seniority`, `open_count`, `growth`, and a `skills` array whose entries carry `skill`,
  `open_count` and `share`, ordered by `open_count` descending

#### Scenario: Share is relative to the skill-bearing sample, not the open count

- **WHEN** a role has 1,000 open postings, 900 of which carry at least one tagged skill,
  and 710 of those carry `docker`
- **THEN** `sample_size` is `900` and that skill's `share` is `710/900`, never `710/1000`

#### Scenario: Sample size is served beside the distribution

- **WHEN** a client requests a single role's skill demand
- **THEN** the response carries `sample_size` alongside `open_count`, and the two are
  distinct fields because they count different things

#### Scenario: Seniority without category is refused

- **WHEN** a client requests `GET /api/v1/insights/roles?seniority=senior` with no
  `category`
- **THEN** the response is `400` with an `{"error": ...}` body, because a seniority across
  every category is not a role

#### Scenario: Invalid seniority rejected

- **WHEN** a client requests
  `GET /api/v1/insights/roles?category=backend&seniority=archmage`
- **THEN** the response is `400` with an `{"error": ...}` body and no partial data

#### Scenario: Seniority is a read parameter, not an ignored one

- **WHEN** a client supplies `seniority`
- **THEN** it never appears in `meta.ignored_params`, because the endpoint reads it

#### Scenario: Ranking is unchanged when no single role is named

- **WHEN** a client requests `GET /api/v1/insights/roles` with no `seniority`
- **THEN** the response is the existing ranked list of roles, and no entry carries a
  `skills` array

#### Scenario: Role below the sample floor carries no skills

- **WHEN** a named role's postings are too few for any skill to clear the worker's sample
  floor
- **THEN** the response is `200` with that role's counts and an empty `skills` array,
  never a `404` and never an unfloored distribution

#### Scenario: Country scopes the counts but not the skills

- **WHEN** a client requests
  `GET /api/v1/insights/roles?category=backend&seniority=senior&country=DE`
- **THEN** `open_count` and `growth` are scoped to `DE`, the `skills` array is the
  country-agnostic distribution, and `meta` states that the skill distribution is not
  geography-scoped

