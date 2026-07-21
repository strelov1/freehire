# job-engagement-counts Specification

## Purpose
TBD - created by archiving change job-view-apply-counts. Update Purpose after archive.
## Requirements
### Requirement: Job carries materialized engagement counters

Each job SHALL carry two non-negative integer counters, `view_count` and
`applied_count`, materialized on the `jobs` row (default `0`). `applied_count` is
the number of **distinct signed-in users** who have marked the job applied.
`view_count` is the number of **distinct daily visitors across all traffic**
(anonymous web, signed-in web, and external API) who have opened the job, as
maintained by the offline nginx-log aggregation worker (see
`view-count-aggregation`). `POST /api/v1/jobs/:slug/view` SHALL NOT increment
`view_count`; it records only the per-user interaction. Read paths SHALL serve
both values directly from the `jobs` row without any per-request counting or join.

#### Scenario: Counters default to zero

- **WHEN** a job has no recorded interactions or views
- **THEN** its `view_count` and `applied_count` are both `0`

#### Scenario: view_count is not bumped on the per-user view beacon

- **WHEN** a signed-in user calls `POST /api/v1/jobs/:slug/view`
- **THEN** the user's `user_jobs.viewed_at` is recorded
- **AND** the job's `view_count` is not changed by that request

#### Scenario: Existing interactions are backfilled on release

- **WHEN** the change is released against a database that already holds
  `user_jobs` rows and retained nginx logs
- **THEN** `applied_count` is set to each job's count of users whose `applied_at`
  is set
- **AND** `view_count` is seeded by the log-aggregation backfill over retained
  history (see `view-count-aggregation`)

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

