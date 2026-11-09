# aggregator-source-providers Specification

## Purpose

The job aggregators the catalogue crawls and what each adapter must do: the boardless ones (Tecla, JobStash, Working Nomads, Himalayas, Remotive), Reed (keyed and keyword-scoped), and Computrabajo, which is board-scoped (one board per country site, such as `ar`). Split out of source-ingest, which holds the rules every adapter shares.

## Requirements

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

### Requirement: Working Nomads is a registered boardless aggregator provider

The `workingnomads` provider SHALL crawl `workingnomads.com` through its public JSON feed
(`/api/exposed_jobs/`), a single flat array of postings with the body inline (no detail
call). Each posting maps to the normalized job shape with the employer taken from the
posting. The feed carries no numeric id field, so the adapter SHALL derive `ExternalID`
from the posting URL path (`/job/go/<id>/`); a posting whose URL yields no id SHALL be
dropped rather than persisted with an empty id.

#### Scenario: Working Nomads posting maps to a job with id from its URL

- **WHEN** a `workingnomads` posting carries URL `.../job/go/1663269/`, a title, a company name, an inline HTML description, and a `pub_date`
- **THEN** the job's `ExternalID` is `1663269`, its `Company` comes from the posting, and `Title`/`Description`/`PostedAt` come from that payload

#### Scenario: Posting with an unparseable URL is dropped

- **WHEN** a `workingnomads` posting has a URL with no `/job/go/<id>/` segment
- **THEN** the adapter drops it and does not emit a job with an empty `ExternalID`

### Requirement: Himalayas is a registered boardless aggregator provider

The `himalayas` provider SHALL crawl `himalayas.app` through its public JSON API
(`/jobs/api`), paginating by offset/limit over the API-reported `totalCount` up to a
defensive maximum-page cap. Each posting maps to the normalized job shape with the employer
taken from `companyName` and the canonical link from `applicationLink`.

#### Scenario: Himalayas is crawled across offset pages

- **WHEN** the `himalayas` provider crawls its boardless entry and the API reports a `totalCount` larger than one page
- **THEN** the adapter requests successive offset pages until the catalogue is exhausted or the defensive page cap is reached
- **AND** every posting becomes a job whose `Company` is its `companyName` and whose `ExternalID` is the posting's stable id (`guid`)

### Requirement: Remotive is a registered boardless aggregator provider

The `remotive` provider SHALL crawl `remotive.com` through its public JSON API
(`/api/remote-jobs`) with a **single** request per run — the API is rate-limited and serves
24h-delayed data, so the adapter SHALL NOT paginate or poll. Each posting maps to the
normalized job shape with the employer from `company_name`; Remotive lists only remote jobs,
so every job SHALL be marked remote.

#### Scenario: Remotive performs a single fetch and marks jobs remote

- **WHEN** the `remotive` provider crawls its boardless entry
- **THEN** the adapter makes exactly one request to the feed (no pagination loop)
- **AND** every returned posting becomes a remote job (`Remote` true, `WorkMode` "remote") with `Company` from `company_name` and `ExternalID` from the posting id

### Requirement: Reed is a registered keyed, keyword-scoped aggregator provider

The ingest registry SHALL include a `reed` adapter over the Reed Jobseeker API
(reed.co.uk), and the crawl registry SHALL include it only when the `REED_API_KEY`
environment variable is set — like `usajobs`, the key is a secret read from the
environment and never stored in a board file. The taxonomy registry SHALL list `reed`
regardless, per "The provider taxonomy is independent of crawl credentials". The adapter
SHALL be boardless (one API, no per-tenant board id) and SHALL declare itself an
`aggregator`, taking each posting's employer from the API payload and remaining a value
in the source facet.

Because the Reed API filters only by free-text keywords (it exposes no sector
filter) and freehire is an IT job board, the adapter SHALL enumerate a topical IT
slice by searching a curated set of IT/technology keywords, unioning the results and
**deduping by the Reed job id** so a posting matched by several keywords is crawled
once. Because the search list omits the employer's real apply URL and truncates the
description, the adapter SHALL fetch each unique job's detail and take the full
description and the employer's `externalUrl` from it, falling back to the Reed
listing URL (`jobUrl`) when no `externalUrl` is present. Authentication SHALL use the
API key as HTTP Basic credentials.

Because the Reed API enforces a per-hour request quota, the adapter SHALL be a
hydrating adapter (per the "Adapters may hydrate only postings the catalogue lacks"
requirement): it SHALL fetch a posting's detail only when that posting is not already
ingested, and SHALL mark an already-ingested posting for a liveness refresh instead of
re-fetching its detail. When the pipeline cannot supply a seen-set, the adapter SHALL
fall back to fetching every unique job's detail.

#### Scenario: Registered only when the key is configured

- **WHEN** `REED_API_KEY` is unset and the crawl registry is assembled
- **THEN** it does NOT contain `reed`
- **AND WHEN** `REED_API_KEY` is set
- **THEN** the crawl registry contains `reed` carrying that key

#### Scenario: Listed in the source facet whatever the environment holds

- **WHEN** the source-facet provider list is built with `REED_API_KEY` unset
- **THEN** `reed` is listed

#### Scenario: Keyword matches are deduped by job id

- **WHEN** the same Reed job id is returned by more than one of the curated
  keyword searches
- **THEN** the adapter emits that posting once, not once per matching keyword

#### Scenario: The employer apply URL comes from the job detail

- **WHEN** a job's detail carries an `externalUrl` (the employer's own posting)
- **THEN** the emitted job's URL is that `externalUrl`, not the Reed listing URL
- **AND WHEN** the detail has no `externalUrl`
- **THEN** the emitted job's URL falls back to the Reed listing URL

#### Scenario: Detail is fetched only for postings not already ingested

- **WHEN** the pipeline supplies a seen-set and a unioned Reed job id is already ingested
- **THEN** the adapter marks that posting for a liveness refresh and issues NO detail request
- **AND WHEN** a unioned Reed job id is not in the seen-set
- **THEN** the adapter fetches that job's detail and emits the hydrated posting

#### Scenario: Falls back to hydrating every posting without a seen-set

- **WHEN** the pipeline cannot supply a seen-set (e.g. a non-DB caller)
- **THEN** the adapter fetches every unique job's detail, as before

### Requirement: Browser-compatible Computrabajo posting hrefs

The Computrabajo adapter MUST remove ASCII tab, carriage return and line feed from
a posting href before validating its path and constructing the detail request URL.

#### Scenario: Embedded control whitespace in a posting slug

- **WHEN** a valid listing card contains tab or newline characters inside its href
- **THEN** its URL is the same URL a browser would request without those characters
- **AND** its posting identity and title remain unchanged
- **AND** the resulting URL can construct an HTTP GET request
