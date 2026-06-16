## ADDED Requirements

### Requirement: An aggregator provider derives each job's company from the posting

An aggregator provider MUST set each job's company from the posting payload itself, not from the configured board entry. An aggregator is a provider that crawls a marketplace whose postings each name a different employer. The configured entry's company field is a placeholder used only to satisfy board-file validation. An aggregator provider MUST be boardless (one global feed, no per-tenant board id), so its entry omits the board id.

#### Scenario: Each posting keeps its own employer

- **WHEN** an aggregator board returns postings whose payloads name employers "Sliiip" and "Psyflo"
- **THEN** the corresponding jobs carry `Company` "Sliiip" and "Psyflo" respectively
- **AND** neither inherits the configured entry's placeholder company

### Requirement: Tecla is a registered boardless aggregator provider

The `tecla` provider SHALL crawl the `app.tecla.io` marketplace through its public JSON API,
paginating `getPublicJobs` over the API-reported page count up to a defensive cap, and map
each posting to the normalized job shape. Tecla is a remote-only marketplace, so every job
SHALL be marked remote. The public API truncates the description; the adapter SHALL persist
that public text as-is (the full, auth-gated text is intentionally not fetched).

#### Scenario: Tecla board is crawled across pages

- **WHEN** the `tecla` provider crawls its boardless entry and the API reports more than one page
- **THEN** the adapter requests each page through the API-reported page count
- **AND** every returned posting becomes a job with `ExternalID` set to the posting id and `URL` pointing at `app.tecla.io/job?id=<id>`

#### Scenario: Tecla posting maps to a remote job with its own employer

- **WHEN** a tecla posting carries a title, an employer name, a created timestamp, and a (truncated) description
- **THEN** the job's `Title`, `Company`, `PostedAt`, and `Description` come from that payload
- **AND** the job is marked remote (`Remote` true, `WorkMode` "remote")
