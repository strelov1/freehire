## ADDED Requirements

### Requirement: A jobs.dover.com link resolves as a single vacancy

The link-source registry SHALL resolve a `jobs.dover.com/<Org>/<uuid>` link directly through
Dover's public posting-detail endpoint — the endpoint the ingest adapter already hydrates
with — instead of falling back to a full board fetch. The employer SHALL be named from the
posting's own client record (`client_name`), never from a humanized link segment. The
resolved posting SHALL be namespaced by the board as the catalogue stores it — the spelling the
crawl pipeline namespaces rows by — or, when the catalogue has no such board or is not available
to the caller, by the link's Org spelling, which the recognizer would file it under — so a
link-imported row and a crawl of that board dedup to one posting; the careers API's slug
lookup (`careers-page-slug/<Org>`) SHALL gate the resolve as the OWNERSHIP check (the host's
shell serves any slug, so the host proves shape only): the lookup's client id SHALL equal the
posting's `client_id`. An unresolvable slug, a slug owned by another client, an empty client
name, an unknown (404) posting, an inactive or private posting are all skips — not retriable
errors. The namespaced id SHALL be the posting id the API returns, whatever case the link
spells its uuid in, and only the exact host (case-insensitive, no `www.`) SHALL match. The registry SHALL NOT trust the link's uuid as a client identity: it is
the posting id (the submitted Deepspatial link's uuid is a posting of client `1aeeba55-…`,
live-verified 2026-11-05).

#### Scenario: The submitted Deepspatial link resolves under the company's identity

- **WHEN** the link-source registry resolves
  `https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d`
- **THEN** the vacancy carries company `Deepspatial` from the posting's client record and
  external id `Deepspatial:a5546acf-45c3-44ff-bedd-a4314fd0f37d`, matching the identity a
  crawl of the `Deepspatial` board writes

#### Scenario: A link spells a catalogued board differently

- **WHEN** the catalogue stores the board as `deepspatial` and the link spells it `Deepspatial`
- **THEN** the external id is `deepspatial:<posting id>`, the row a crawl of that board writes

#### Scenario: An Org that does not own the posting is declined

- **WHEN** a link pairs a real company's Org segment with a posting of a different client
- **THEN** the resolve is a skip, so no row is filed under a board whose crawl never writes it

#### Scenario: An upper-case uuid link dedups with the crawl

- **WHEN** the link spells the posting uuid in upper case
- **THEN** the external id carries the API's lower-case posting id, as the crawl's row does

#### Scenario: Non-job shapes are declined

- **WHEN** a `jobs.dover.com` link without a UUID-shaped second segment (the marketing shell
  or the vendor's `/startup-jobs` listing) reaches the registry
- **THEN** the Dover adapter does not match it, leaving the link to the ordinary resolver
  chain
