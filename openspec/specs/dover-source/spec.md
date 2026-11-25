# dover-source Specification

## Purpose

Crawls Dover-hosted company career pages (app.dover.com) into the job catalogue via Dover's
public, unauthenticated JSON API — board resolution, paginated listing, per-posting hydration.
## Requirements
### Requirement: Board-scoped crawl

The system SHALL provide a `dover` source adapter that crawls one Dover-hosted company's public
career page into the catalogue. The board id is the company's Dover slug (the path segment in
`https://app.dover.com/jobs/<slug>` and `https://app.dover.com/apply/<slug>/<job-id>`, e.g.
`qompyl`). The adapter SHALL resolve the slug to the platform's internal client identifier before
listing that client's jobs, and SHALL return one `Job` per open posting the board currently
publishes.

#### Scenario: A board yields all its published, non-sample postings

- **WHEN** the adapter crawls a configured Dover board whose company currently has several
  published postings and no sample or unpublished ones
- **THEN** it returns one `Job` per published posting

#### Scenario: An unresolvable slug fails the board without aborting other boards

- **WHEN** a configured board's slug no longer resolves to a Dover company
- **THEN** the adapter reports that board's crawl as failed and yields no postings for it, while
  every other configured board still crawls normally

### Requirement: Listing is paginated to completion

The Dover job-listing endpoint returns pages of results; the adapter SHALL follow pagination until
every page for the board's client has been read, and SHALL NOT stop at the first page when more
remain.

#### Scenario: A board with more postings than one page's size is fully listed

- **WHEN** a board's client has more open postings than fit in a single listing page
- **THEN** the adapter fetches every page and returns postings from all of them, not only the first

### Requirement: Sample, unpublished and inactive postings are excluded

The adapter SHALL exclude a posting the listing marks as a sample, a posting not marked as
published, and a posting whose detail marks it inactive or private, from every crawl's results.

#### Scenario: A sample posting is never returned

- **WHEN** a board's listing includes a posting flagged as a sample

- **THEN** the adapter does not return a `Job` for that posting

#### Scenario: An unpublished posting is never returned

- **WHEN** a board's listing includes a posting not marked as published
- **THEN** the adapter does not return a `Job` for that posting

#### Scenario: An inactive or private posting is dropped after detail fetch

- **WHEN** a posting passes the listing filters but its detail fetch reports it inactive or private
- **THEN** the adapter drops that posting rather than yielding a `Job` for it

### Requirement: Per-posting hydration

For each surviving posting, the adapter SHALL fetch that posting's own detail and map: the
platform's job identifier to `ExternalID`; the constructed `https://app.dover.com/apply/<slug>/<job-id>`
address to the posting URL; the title; the board's configured employer name to the posting's
company, consistent with every other single-employer boarded adapter; the HTML body to a sanitized
description; and the stated work arrangement (remote, onsite, or hybrid) and any stated
country/region to the posting's location and work-mode fields. A posting
whose detail fetch fails SHALL still be returned as a list-only `Job` (identity and whatever the
listing itself stated, with no description), rather than being dropped from the crawl outright —
the listing already carries enough identity to be worth keeping, and the catalogue's own
hydration-retry mechanism revisits a stored posting with no description on a later crawl.

#### Scenario: A posting with full detail maps completely

- **WHEN** a posting's detail fetch succeeds and states a title, description, work arrangement, and
  at least one location
- **THEN** the resulting `Job` carries all of them

#### Scenario: A failed detail fetch still yields a list-only posting

- **WHEN** one posting's detail fetch fails while the rest of the board's postings fetch
  successfully
- **THEN** that posting is still returned as a `Job` carrying its listing identity with no
  description, and every other posting on the board is still returned in full

### Requirement: Salary published only when the employer opted in

The adapter SHALL set a posting's published salary range only when the posting's own compensation
data is marked as opted in to public sharing, and SHALL leave the range unset otherwise — even when
the platform's internal data carries bounds for a non-shared posting.

#### Scenario: An opted-in compensation range is published

- **WHEN** a posting's compensation is marked as shared publicly and states a lower and upper bound
- **THEN** the resulting `Job` carries that range and its currency

#### Scenario: A non-shared compensation range is never published

- **WHEN** a posting's compensation is not marked as shared publicly
- **THEN** the resulting `Job` carries no salary range, regardless of what bounds the platform
  states internally

### Requirement: Facts without a structured field are folded into the description

For a posting fact the platform states but freehire's `Job` shape has no dedicated field for
(equity-only or equity-inclusive compensation, and visa sponsorship), the adapter SHALL fold that
fact into the posting's description text rather than discarding it. Employment type is excluded
from this rule: `Job.EmploymentType` is a real structured field, so a stated employment type is
mapped onto it directly (see the posting-normalization requirement) rather than duplicated into
prose.

#### Scenario: An equity-only posting states that fact in its description

- **WHEN** a posting's compensation offers equity with no salary
- **THEN** the resulting `Job`'s description states the equity-only nature of the compensation

#### Scenario: Visa sponsorship is stated in the description

- **WHEN** a posting states that its employer sponsors a work visa
- **THEN** the resulting `Job`'s description includes that fact

### Requirement: Stable dedup identity

The adapter SHALL set each `Job`'s `ExternalID` to the platform's own job identifier, so re-crawling
the same board dedups to the same catalogue row across runs.

#### Scenario: Re-crawl dedups to one row

- **WHEN** the same posting is crawled on two separate runs
- **THEN** both map to the same `ExternalID` and upsert to a single catalogue row

### Requirement: A jobs.dover.com link resolves as a single vacancy

The link-source registry SHALL resolve a `jobs.dover.com/<Org>/<uuid>` link directly through
Dover's public posting-detail endpoint — the endpoint the ingest adapter already hydrates
with — instead of falling back to a full board fetch. The employer SHALL be named from the
posting's own client record (`client_name`), never from a humanized link segment. The
resolved posting SHALL be namespaced by the board as the catalogue stores it — the spelling the
crawl pipeline namespaces rows by — or, when the catalogue has no such board, by the link's Org
spelling, which the recognizer would file it under — so a
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

