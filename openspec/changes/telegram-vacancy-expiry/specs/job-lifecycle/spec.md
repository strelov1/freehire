## ADDED Requirements

### Requirement: A job from a source with no close signal is closed by age

The system SHALL close an open job whose source carries no close signal at all — neither a
re-crawl that could stop seeing it, nor a change feed, nor a posting URL a probe could reach
a verdict on — once its effective posting date (`COALESCE(posted_at, created_at)`) is older
than a fixed window of 45 days. The rule SHALL apply ONLY to that set of sources, and SHALL
NOT apply to a source the ingest sweep or the liveness probe already covers, since for those
a close rests on evidence and age would override it with a guess. An age close SHALL NOT
reopen the job, because no re-crawl exists that could reopen it.

#### Scenario: A stale Telegram vacancy is closed

- **WHEN** the liveness worker runs and an open job has `source = 'telegram'` with an
  effective posting date 46 days in the past
- **THEN** the job is closed with reason `expired`

#### Scenario: A recent Telegram vacancy is left open

- **WHEN** the liveness worker runs and an open job has `source = 'telegram'` with an
  effective posting date 44 days in the past
- **THEN** the job stays open

#### Scenario: Age does not close a job a sweep or probe covers

- **WHEN** the liveness worker runs and an open job has `source = 'greenhouse'` or
  `source = 'manual'` with an effective posting date a year in the past
- **THEN** the age rule does not close it

### Requirement: Every close records the mechanism that wrote it

The system SHALL record, alongside `closed_at`, which mechanism closed the job: the unseen
sweep, a feed's removal event, a moderator, the liveness probe, or the age rule. A job that
reopens SHALL have that record cleared together with `closed_at`, so a reopened job never
carries the label of the mechanism that closed it. A job closed before this requirement
existed SHALL carry an empty record, which reads as "unknown" and SHALL NOT be inferred
after the fact.

#### Scenario: Each mechanism labels its own close

- **WHEN** a job is closed by the unseen sweep, and another by the liveness probe, and
  another by the age rule
- **THEN** each carries a distinct reason identifying the mechanism that closed it

#### Scenario: A reopened job carries no close reason

- **WHEN** a closed job is re-ingested and reopens
- **THEN** both its `closed_at` and its close reason are cleared

## MODIFIED Requirements

### Requirement: Orphan jobs are liveness-probed by URL

The system SHALL probe the posting URL of every open job whose `source` is not a
registered ATS board provider — the sources no ingest run re-crawls (e.g.
`manual`, `habr_career`, `geekjob`). Board-provider jobs, which the ingest sweep
already covers, SHALL NOT be probed. A source whose stored URL is a container that
outlives the vacancy rather than the vacancy's own page SHALL also be excluded, since
no probe of it can reach a death verdict; `telegram` is such a source, its URL being
the post, and it is closed by the age rule instead. The probe SHALL use a plain HTTP
request (no headless browser, no LLM) with a per-probe timeout, and SHALL classify the
outcome without any persisted page content.

#### Scenario: Orphan job is a probe candidate

- **WHEN** the liveness worker runs and an open job has `source = 'manual'`
- **THEN** that job's posting URL is fetched and classified

#### Scenario: Board job is not probed

- **WHEN** the liveness worker runs and an open job has `source = 'greenhouse'`
  (a registered ATS provider)
- **THEN** that job is not selected for probing

#### Scenario: A source whose URL outlives the vacancy is not probed

- **WHEN** the liveness worker runs and an open job has `source = 'telegram'`
- **THEN** that job is not selected for probing

#### Scenario: Closed job is not probed

- **WHEN** the liveness worker runs and an orphan job already has `closed_at` set
- **THEN** that job is not selected for probing
