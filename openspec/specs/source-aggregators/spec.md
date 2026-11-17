# source-aggregators Specification

## Purpose
Providers that are not one employer's board: boardless providers, aggregators that derive each job's company, multi-company aggregators in the source facet, and community-hub boards. Split out of source-ingest.

## Requirements

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

### Requirement: An aggregator provider derives each job's company from the posting

An aggregator provider MUST set each job's company from the posting payload itself, not from the configured board entry. An aggregator is a provider that crawls a marketplace whose postings each name a different employer. The configured entry's company field is a placeholder used only to satisfy board-file validation. An aggregator provider MUST be boardless (one global feed, no per-tenant board id), so its entry omits the board id.

#### Scenario: Each posting keeps its own employer

- **WHEN** an aggregator board returns postings whose payloads name employers "Sliiip" and "Psyflo"
- **THEN** the corresponding jobs carry `Company` "Sliiip" and "Psyflo" respectively
- **AND** neither inherits the configured entry's placeholder company

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

### Requirement: A board entry MAY be marked as a community hub whose employer comes from each posting

The system SHALL support an optional `hub` flag on a board-file entry that marks a board-based
provider's board as a community/agency hub: a board that lists vacancies on behalf of many
partner companies rather than a single employer. Unlike a boardless aggregator (one global feed,
no board id), a hub entry still names a board id and still requires a `company` — that configured
company is the hub's own name and the per-vacancy fallback employer. When the flag is absent or
false, the provider's existing behaviour SHALL be unchanged and each job's company SHALL be the
configured entry company.

For a hub entry, the provider SHALL resolve each vacancy's employer from the posting itself
rather than from the configured company, falling back to the configured company when the posting
carries no resolvable employer (for example the hub's own internal roles).

A hub entry MAY additionally carry an optional `tenants` map, from the tenant key the platform
stamps on a posting to that tenant's employer display name. It exists for hubs whose postings
identify the tenant only by an opaque key — a URL path segment or an id — which is not itself a
usable company name. The map SHALL be optional and SHALL be ignored by providers that do not
implement hub resolution, exactly as `hub` and `region` are. A tenant key absent from the map
SHALL fall back to the configured company rather than be turned into a name by transforming the
key, because a plausible-but-wrong employer is worse in the catalogue than the parent brand.

#### Scenario: A non-hub entry attributes every job to the configured company

- **WHEN** a board file lists an entry without the `hub` flag (or with `hub: false`)
- **THEN** each crawled job's company is the entry's configured `company`, exactly as before

#### Scenario: A hub entry attributes each job to the employer named in the posting

- **WHEN** a board file lists an entry with `hub: true`
- **THEN** each crawled job's company is resolved from that vacancy's own payload, and only a
  vacancy with no resolvable employer falls back to the configured `company`

#### Scenario: A hub entry maps an opaque tenant key to an employer name

- **WHEN** a hub entry carries a `tenants` map and a vacancy's tenant key is present in it
- **THEN** the job's company is that map's value

#### Scenario: An unmapped tenant key falls back rather than being invented

- **WHEN** a hub entry carries a `tenants` map and a vacancy's tenant key is absent from it
- **THEN** the job's company is the configured `company`, and the key is NOT transformed into a
  name
