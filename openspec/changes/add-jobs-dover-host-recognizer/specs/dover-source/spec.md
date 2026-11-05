## ADDED Requirements

### Requirement: A jobs.dover.com-spelled board crawls through the slug path

The system SHALL crawl a board spelled by a `jobs.dover.com` link exactly like any other
Dover board: the board id is the company's slug, the adapter resolves it through the existing
`careers-page-slug` lookup before listing, and every posting it yields carries the platform's
own apply-URL shape `https://app.dover.com/apply/<slug>/<job-id>`. The adapter SHALL NOT
trust any uuid a harvested link carried as a client identity — such a uuid may be a posting
id (the submitted Deepspatial link's is), and the slug lookup is both the supported address
path and the existence check the host's marketing shell cannot provide.

#### Scenario: A jobs.dover.com-spelled board crawls like an app.dover.com board

- **WHEN** the adapter crawls the board `Deepspatial`, filed from a
  `jobs.dover.com/Deepspatial/<uuid>` link, whose slug resolves to client `1aeeba55-…`
- **THEN** it lists `careers-page/1aeeba55-…/jobs` and yields postings whose URLs are
  `https://app.dover.com/apply/Deepspatial/<job-id>`
