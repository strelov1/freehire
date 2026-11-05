## MODIFIED Requirements

### Requirement: Board-scoped crawl

The system SHALL provide a `dover` source adapter that crawls one Dover-hosted company's public
career page into the catalogue. The board id is EITHER the company's Dover slug (the path
segment in `https://app.dover.com/jobs/<slug>` and `https://app.dover.com/apply/<slug>/<job-id>`,
e.g. `qompyl`) OR the `jobs.dover.com` pair `<Org>/<client-uuid>` (see the recognizer's board
requirement). For a slug board the adapter SHALL resolve the slug to the platform's internal
client identifier before listing; for a pair board the pair already carries that identifier and
the adapter SHALL list under it directly, without a slug lookup. In both cases the adapter
SHALL return one `Job` per open posting the board currently publishes.

#### Scenario: A board yields all its published, non-sample postings

- **WHEN** the adapter crawls a configured Dover board whose company currently has several
  published postings and no sample or unpublished ones
- **THEN** it returns one `Job` per published posting

#### Scenario: An unresolvable slug fails the board without aborting other boards

- **WHEN** a configured board's slug no longer resolves to a Dover company
- **THEN** the adapter reports that board's crawl as failed and yields no postings for it, while
  every other configured board still crawls normally

#### Scenario: A pair board lists directly under its uuid

- **WHEN** the adapter crawls a board whose id is the `jobs.dover.com` pair
  `<Org>/<client-uuid>`
- **THEN** it pages the listing endpoint under that client uuid and performs no
  `careers-page-slug` lookup
