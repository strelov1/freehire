## ADDED Requirements

### Requirement: A jobs.dover.com link resolves as a single vacancy

The link-source registry SHALL resolve a `jobs.dover.com/<Org>/<uuid>` link directly through
Dover's public posting-detail endpoint — the endpoint the ingest adapter already hydrates
with — instead of falling back to a full board fetch. The employer SHALL be named from the
posting's own client record (`client_name`), never from a humanized link segment, and the
resolved posting SHALL be namespaced under the canonical slug the careers API reports for
that company (`careers-page-slug/<client_name>`), so a link-imported row and a crawl of the
board the contribution flow files this link under dedup to one posting. The registry SHALL
NOT trust the link's uuid as a client identity: it is the posting id (the submitted
Deepspatial link's uuid is a posting of client `1aeeba55-…`, live-verified 2026-11-05).

#### Scenario: The submitted Deepspatial link resolves under the company's identity

- **WHEN** the link-source registry resolves
  `https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d`
- **THEN** the vacancy carries company `Deepspatial` from the posting's client record and
  external id `deepspatial:a5546acf-45c3-44ff-bedd-a4314fd0f37d`, matching the identity a
  crawl of the `deepspatial` board writes

#### Scenario: Non-job shapes are declined

- **WHEN** a `jobs.dover.com` link without a UUID-shaped second segment (the marketing shell
  or the vendor's `/startup-jobs` listing) reaches the registry
- **THEN** the Dover adapter does not match it, leaving the link to the ordinary resolver
  chain
