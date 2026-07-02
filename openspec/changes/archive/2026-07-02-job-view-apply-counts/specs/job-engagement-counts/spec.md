## ADDED Requirements

### Requirement: Job carries materialized engagement counters

Each job SHALL carry two non-negative integer counters, `view_count` and
`applied_count`, materialized on the `jobs` row (default `0`). The counters count
**distinct signed-in users**: `view_count` is the number of distinct signed-in
users who have opened the job's detail page; `applied_count` is the number of
distinct users who have marked the job applied. Read paths SHALL serve these
values directly from the `jobs` row without any per-request counting or join.

#### Scenario: Counters default to zero

- **WHEN** a job has no recorded interactions
- **THEN** its `view_count` and `applied_count` are both `0`

#### Scenario: Existing interactions are backfilled on release

- **WHEN** the migration that introduces the counters runs against a database
  that already holds `user_jobs` rows
- **THEN** each job's `view_count` is set to its count of interacting users and
  `applied_count` to its count of users whose `applied_at` is set

### Requirement: Job wire shape exposes the counters

The public job wire shape SHALL expose `view_count` and `applied_count` as
integer fields, populated from the `jobs` row on every job read (list, detail,
search).

#### Scenario: Detail response includes the counters

- **WHEN** a client requests `GET /api/v1/jobs/:slug`
- **THEN** the `data` object includes integer `view_count` and `applied_count`
  fields reflecting the stored counters

### Requirement: SPA displays the counters on the job detail page

The job detail page SHALL display the job's view and apply counts. A counter that
is `0` SHALL be omitted so the display never reads as a dead "0 views". The counts
are shown to every visitor, signed in or not.

#### Scenario: Counts shown on a job with engagement

- **WHEN** a visitor opens a job whose `view_count` is 5 and `applied_count` is 2
- **THEN** the detail page shows both the view count and the apply count

#### Scenario: Zero counters are omitted

- **WHEN** a visitor opens a job whose `applied_count` is 0
- **THEN** the apply count is not rendered
