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

The job card SHALL additionally display the view count in its header rail, beside
the relative posting timestamp, so the browse feed conveys how much attention a
posting has drawn without opening it. The card SHALL show `view_count` only —
`applied_count` is freehire's own tracking marker and is near zero across the
catalogue, so it would read as a dead figure on every card. The same
zero-omission rule applies: a card whose `view_count` is `0` SHALL render no
count at all rather than a "0".

Counts of a thousand or more SHALL be abbreviated on the card (for example
`1.2K`), because the rail shares its width with the company name and the
timestamp and an unabbreviated five-digit figure crowds both out. The detail
page, which has the room, keeps the exact figure.

Listing shapes that do not carry `view_count` — the tracking and assistant card
projections — SHALL render no count, the same way they already render no salary.

#### Scenario: Counts shown on a job with engagement

- **WHEN** a visitor opens a job whose `view_count` is 5 and `applied_count` is 2
- **THEN** the detail page shows both the view count and the apply count

#### Scenario: Zero counters are omitted

- **WHEN** a visitor opens a job whose `applied_count` is 0
- **THEN** the apply count is not rendered

#### Scenario: The card shows the view count beside the timestamp

- **WHEN** a job card is rendered for a job whose `view_count` is 231
- **THEN** the card's header rail shows the view count beside the relative
  posting timestamp

#### Scenario: The card omits a zero view count

- **WHEN** a job card is rendered for a job whose `view_count` is 0
- **THEN** no view count is rendered on the card

#### Scenario: The card never shows the apply count

- **WHEN** a job card is rendered for a job whose `applied_count` is 4
- **THEN** no apply count is rendered on the card

#### Scenario: Large counts are abbreviated on the card

- **WHEN** a job card is rendered for a job whose `view_count` is 1240
- **THEN** the card shows an abbreviated figure such as `1.2K`

#### Scenario: A listing card projection without the counter renders nothing

- **WHEN** a card is rendered from a listing projection that carries no
  `view_count` field
- **THEN** no view count is rendered and the card is otherwise unchanged

