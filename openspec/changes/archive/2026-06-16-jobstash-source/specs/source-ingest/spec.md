# source-ingest (delta)

## MODIFIED Requirements

### Requirement: A boardless provider may omit its board id

A source adapter SHALL be able to declare itself `boardless` when its API has no
per-tenant board id, so its configured entries MAY omit `board`. A `boardless`
provider MAY serve exactly one company or MAY aggregate postings from many
companies; the marker concerns only the absence of a board id, not the number of
companies. A provider that has a board or tenant concept SHALL NOT be boardless
and SHALL continue to require a non-empty `board`; this includes `yandex`, whose
`board` selects host and language.

#### Scenario: Boardless adapter is dispatched without a board

- **WHEN** a `boardless` provider's entry is crawled with an empty `board`
- **THEN** the adapter fetches its postings without requiring a board id and
  yields the normalized job shape

#### Scenario: Yandex requires its board

- **WHEN** a `yandex` entry is configured with an empty `board`
- **THEN** validation fails fast, because `yandex` selects host and language (`ru`/`com`)
  by its `board` and is not boardless

## ADDED Requirements

### Requirement: A multi-company aggregator stays in the source facet

The source facet (the provider list a user filters results by) SHALL exclude a
`boardless` provider that serves a single company, because filtering by it is
redundant with the company filter. A `boardless` provider that aggregates
postings from many companies SHALL declare itself an `aggregator` and SHALL
remain a value in the source facet, because filtering by "this aggregator" is not
redundant with any single company. A provider that has a board concept (not
boardless) SHALL remain in the source facet as before.

#### Scenario: Single-company boardless provider is excluded from the source facet

- **WHEN** the source-facet provider list is built (e.g. for the web contract)
- **THEN** a single-company boardless provider such as `ozon` is not listed

#### Scenario: Aggregator boardless provider stays in the source facet

- **WHEN** the source-facet provider list is built
- **THEN** an `aggregator` boardless provider such as `jobstash` or the existing
  `tecla` marketplace is listed, while the board-based providers (`greenhouse`,
  `lever`, …) remain listed as before

### Requirement: JobStash crypto-job aggregator is registered

The system SHALL register a `jobstash` adapter for the JobStash Web3 job
aggregator, configurable as a single `boardless` entry. The adapter SHALL
paginate JobStash's job-list endpoint by the reported total and yield every
posting as the normalized job shape (at least title, url, location, remote flag,
description, and the platform's native posting id). The company of each posting
SHALL be taken from the posting's `organization.name`, so one board yields jobs
across many companies. A posting carrying a `url` (a `public` posting's downstream
ATS apply link) SHALL use it unchanged; a posting with no `url` (a `protected`
posting, whose gated link the feed omits) SHALL link to the JobStash detail page
derived from its native id, so the stored job links to the real apply target when
one exists and to the best available link otherwise. A posting with no native id
or no company SHALL be dropped rather than persisted, since it would break the
dedup key or the company slug. The structured location type
(`ONSITE`/`REMOTE`/`HYBRID`) SHALL set the structured `work_mode`, and the publish
timestamp SHALL set `posted_at`. The adapter SHALL be both `boardless` and an
`aggregator`.

#### Scenario: JobStash board is fully crawled across companies

- **WHEN** a `jobstash` entry is crawled with an empty `board`
- **THEN** the adapter walks the list endpoint to the reported total and yields
  each posting as the normalized job shape, with `company` taken from the
  posting's `organization.name` and `external_id` set to the posting's native id

#### Scenario: A public posting links to its downstream ATS

- **WHEN** JobStash returns a posting whose `access` is `public` with a downstream
  ATS `url`
- **THEN** the yielded job's `url` is that downstream ATS link

#### Scenario: A protected posting with no url links to its JobStash page

- **WHEN** JobStash returns a `protected` posting whose `url` is null
- **THEN** the yielded job's `url` is the JobStash detail page derived from the
  posting's native id

#### Scenario: A posting with no company or id is dropped

- **WHEN** JobStash returns a posting with an empty `organization.name` or an
  empty native id
- **THEN** that posting is not yielded, while the other postings are

#### Scenario: Location type sets the structured work mode

- **WHEN** a posting reports `locationType` `REMOTE`
- **THEN** the yielded job's structured `work_mode` is `remote`
