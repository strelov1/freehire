# enterprise-ats-providers Specification

## Purpose

Enterprise and single-employer ATS providers (SuccessFactors, Google, Eightfold, Workday, Meta) and their hydration and hub-employer rules. Split out of source-ingest, which holds the rules every adapter shares.

## Requirements

### Requirement: successfactors is a registered provider

The system SHALL register a `successfactors` adapter so SAP SuccessFactors career sites
can be listed in a `sources/` board file. The adapter SHALL treat the configured `board` value as the
career-site host and enumerate jobs from that site's `GET https://<board>/job_sitemap.xml`,
taking each `<url>`'s `<loc>` as the job page URL (with the job's native id as the numeric
segment of that path) and its `<lastmod>` as the posting date. Because the sitemap carries
no description, the adapter SHALL fetch each job page and extract the title and description
from the page's schema.org JobPosting microdata (`itemprop="title"` and
`itemprop="description"`), with bounded concurrency; a single failed page fetch SHALL drop
only that posting rather than abort the board. The adapter SHALL yield the normalized job
shape (at least title, url, remote flag, description, and the platform's native posting id),
with the `description` as sanitized HTML, consistent with the existing adapters. The job
`location` MAY be empty, since SuccessFactors does not expose it in the microdata and
enrichment derives it from the description.

#### Scenario: SuccessFactors board is enumerated from its sitemap

- **WHEN** a `sources/` board file lists a board with provider `successfactors` and a career-site host
- **THEN** the adapter fetches `https://<host>/job_sitemap.xml`, and per `<loc>` fetches the
  job page, yielding each as the normalized job shape with `external_id` set to the numeric
  id from the job URL and `posted_at` derived from the entry's `<lastmod>`

#### Scenario: Title and description come from JobPosting microdata

- **WHEN** a SuccessFactors job page is fetched
- **THEN** the adapter yields the job's title from `itemprop="title"` and a sanitized HTML
  description from the inner markup of `itemprop="description"`

#### Scenario: A failed job-page fetch drops only that posting

- **WHEN** a board's sitemap lists several jobs and one job page's fetch fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: An empty sitemap yields no jobs without error

- **WHEN** a board's `job_sitemap.xml` lists no job URLs
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: Google is a registered provider

The system SHALL register a `google` adapter so Google's own careers catalogue
(`www.google.com/about/careers/applications/jobs/results`) can be listed in a
`sources/*.yml` board file. The adapter SHALL be **boardless** (a single-company source
with no per-tenant board id), like the existing `uber`/`amazon` adapters. It SHALL fetch the
server-rendered list pages over the shared `HTTPClient`, paging via the `?page=N` query
parameter, and SHALL read each posting from the JSON payload embedded in the page's
`AF_initDataCallback({key:'ds:1', data:[…]})` script block — without any per-job detail
request, since that payload already carries the full job text. The adapter SHALL yield the
normalized job shape (at least title, url, location, description, and the platform's native
posting id), with `external_id` set to Google's numeric job id, `url` set to the public
`…/jobs/results/<id>` results page built from the job id (Google ignores any trailing slug,
so none is reproduced; NOT the sign-in-gated apply link), and `description` as sanitized HTML
assembled from the posting's description, responsibilities, and qualifications fields.

#### Scenario: Google catalogue is crawled page by page

- **WHEN** a `sources/*.yml` board lists provider `google`
- **THEN** the adapter requests `…/jobs/results?page=1`, extracts the `ds:1` payload, yields
  each embedded job record as the normalized job shape, then requests the next page, and
  continues until a page yields no records or the running yielded count reaches the payload's
  total-count field

#### Scenario: Job is mapped from the embedded record

- **WHEN** a page's `ds:1` payload carries a job record
- **THEN** the adapter yields a job whose `external_id` is the record's numeric id, whose
  `url` is the public `…/jobs/results/<id>` page built from the id alone, whose `title`,
  `location` (from the structured locations array), and `posted_at`
  (from the embedded Unix-epoch timestamp) come from the record, and whose `description` is
  sanitized HTML assembled from the record's description, responsibilities, and qualifications
  fields

#### Scenario: A posting is dated from its embedded timestamp

- **WHEN** a Google job record carries a Unix-epoch publish timestamp
- **THEN** the adapter yields the job with `posted_at` derived from that timestamp, and
  yields a nil `posted_at` when the timestamp is absent or zero

#### Scenario: An empty or out-of-range page ends the crawl without error

- **WHEN** a requested page past the last page returns no `ds:1` job records (the payload's
  job list is null or empty)
- **THEN** the adapter stops paging, yields the jobs collected so far, and returns no error

### Requirement: Eightfold is a registered provider

The system SHALL register an `eightfold` adapter so an Eightfold-hosted careers catalogue
(e.g. Microsoft's `apply.careers.microsoft.com`) can be listed in a `sources/*.yml` board
file. The adapter SHALL be **board-based** (not boardless): its board id SHALL be
`"host/domain"` — the public host used for request paths and the required `domain` query
parameter (the Eightfold tenant key) — and the adapter SHALL reject a board missing either
half. It SHALL fetch postings over the shared `HTTPClient`. Because Eightfold runs two list-API
generations and a tenant supports exactly one, the adapter SHALL auto-detect: it SHALL try the
newer position list `GET https://<host>/api/pcsx/search?domain=<domain>&query=&start=<n>&num=10`
and, if that fails, fall back to the legacy list
`GET https://<host>/api/apply/v2/jobs?domain=<domain>&query=&start=<n>&num=10`. Because the list
omits the description, the adapter SHALL fetch each position's detail
`GET https://<host>/api/apply/v2/jobs/<id>?domain=<domain>` (shared by both generations). The adapter SHALL yield the
normalized job shape (at least title, url, location, description, and the platform's native
posting id), with `external_id` set to the position's numeric id, `url` set to the detail's
`canonicalPositionUrl` (falling back to `https://<host>/careers/job/<id>`), `description` as
sanitized HTML from the detail's `job_description`, `posted_at` from the list position's
Unix-epoch `postedTs`, and `work_mode` derived from the list position's `workLocationOption`.

#### Scenario: Eightfold catalogue is crawled page by page

- **WHEN** a `sources/*.yml` board lists provider `eightfold` with board `"host/domain"`
- **THEN** the adapter requests
  `https://<host>/api/pcsx/search?domain=<domain>&query=&start=0&num=10&sort_by=relevance`,
  reads `data.positions`, advances `start` by the number of positions returned, and continues
  until a page yields no positions or the running yielded count reaches `data.count`

#### Scenario: Job is assembled from the list position and its detail

- **WHEN** a position from `/api/pcsx/search` is processed
- **THEN** the adapter requests that position's `/api/apply/v2/jobs/<id>?domain=<domain>` detail
  and yields a job whose `external_id` is the position's numeric id, whose `title`, `location`
  (first of the position's `locations`), `posted_at` (from `postedTs`), and `work_mode` (from
  `workLocationOption`) come from the list position, whose `url` is the detail's
  `canonicalPositionUrl` (or `https://<host>/careers/job/<id>` when that is absent), and whose
  `description` is sanitized HTML from the detail's `job_description`

#### Scenario: A legacy tenant falls back to the v2 list

- **WHEN** a board's `/api/pcsx/search` request fails (a legacy tenant returns `403`)
- **THEN** the adapter falls back to `GET https://<host>/api/apply/v2/jobs?domain=<domain>&…`,
  reads the top-level `positions`/`count`, and maps each position (taking `posted_at` from
  `t_create`, `location` from the single-string `location` field, and `work_mode` from
  `work_location_option`) — yielding the same normalized job shape as the pcsx generation

#### Scenario: A failed detail request drops only that posting

- **WHEN** one position's detail request fails while crawling a board
- **THEN** the adapter skips that single posting and still yields the remaining postings,
  without aborting the board

#### Scenario: A board missing the host or domain half is rejected

- **WHEN** an `eightfold` board id is not of the form `"host/domain"` (no `/`, or an empty host
  or domain)
- **THEN** `Fetch` returns an error rather than issuing a malformed request

### Requirement: meta is a registered boardless provider served over a Chrome-fingerprint transport

The system SHALL register a `meta` adapter so Meta's career site (metacareers.com) can be listed
as a boardless single-company source (one `sources/custom.yml` entry with `company: Meta`,
`provider: meta`, and no board), like the other single-company adapters. The adapter SHALL
enumerate jobs from `GET https://www.metacareers.com/jobsearch/sitemap.xml`, taking each `<url>`'s
`<loc>` as a `job_details/<id>` job page URL (with the job's native id as the numeric segment of
that path) and its `<lastmod>` as a fallback posting date. Because the sitemap carries no
description, the adapter SHALL fetch each job page and extract the title, description, posting
date, and location from the page's `application/ld+json` `JobPosting`, with bounded concurrency; a
single failed page fetch SHALL drop only that posting rather than abort the board.

Because Meta's edge rejects the standard Go TLS fingerprint (returning `HTTP 400` to a plain
client while a real browser succeeds), the `meta` adapter SHALL be served by an HTTP client that
presents a Chrome TLS fingerprint. That Chrome-fingerprint client SHALL be scoped to the `meta`
adapter only — the other registered adapters SHALL continue to use the standard shared client
unchanged — and SHALL still route its connections through the SSRF guard (refusing
internal/metadata targets), so the new transport is not an SSRF regression.

The adapter SHALL yield the normalized job shape (at least title, url, location, remote flag,
description, and the platform's native posting id), with the `description` as sanitized HTML and
the `location` taken from the JobPosting's `jobLocation[].name` (never the `jobLocation[].address`
sub-object, whose locality/region/country fields Meta renders incorrectly).

#### Scenario: Meta is enumerated from its jobsearch sitemap over the Chrome-fingerprint client

- **WHEN** `sources/custom.yml` lists a boardless entry with provider `meta`
- **THEN** the adapter fetches `https://www.metacareers.com/jobsearch/sitemap.xml` over the
  Chrome-fingerprint client, and per `<loc>` fetches the job page, yielding each as the normalized
  job shape with `external_id` set to the numeric id from the `job_details/<id>` URL

#### Scenario: Title, description, date, and location come from the ld+json JobPosting

- **WHEN** a Meta job page is fetched
- **THEN** the adapter yields the job's title and a sanitized HTML description from the page's
  `application/ld+json` `JobPosting`, `posted_at` from its `datePosted` (falling back to the
  sitemap entry's `<lastmod>`), and `location` from the first `jobLocation[].name`

#### Scenario: The Chrome-fingerprint transport is scoped to meta and keeps the SSRF guard

- **WHEN** the source registry is assembled
- **THEN** only the `meta` adapter receives the Chrome-fingerprint client, every other adapter
  receives the standard shared client, and the Chrome-fingerprint client still refuses
  internal/metadata network targets

#### Scenario: A failed job-page fetch drops only that posting

- **WHEN** the sitemap lists several jobs and one job page's fetch fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: An empty sitemap yields no jobs without error

- **WHEN** the jobsearch sitemap lists no job URLs
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: The workday adapter hydrates only postings the catalogue lacks

The `workday` adapter SHALL implement the hydrating capability, because a Workday board carries
one posting per detail request and a large board (over twenty thousand postings) is rate-limited
into failure when every crawl re-fetches all of them. It SHALL list a board exactly as the
non-hydrating path does — the listing page size is fixed by the platform and a larger requested
limit yields no postings — and SHALL issue a detail request only for a posting the seen-set does
not contain, marking an already-ingested posting for liveness refresh instead. When the pipeline
cannot supply a seen-set, the adapter SHALL fall back to fetching every posting's detail, as
before.

#### Scenario: Detail is fetched only for postings not already ingested

- **WHEN** the pipeline supplies a seen-set and a listed posting's namespaced id is already
  ingested
- **THEN** the adapter marks that posting for a liveness refresh and issues NO detail request
- **AND WHEN** a listed posting is not in the seen-set
- **THEN** the adapter fetches that posting's detail and emits the hydrated posting

#### Scenario: A liveness refresh carries the posting's identity

- **WHEN** the adapter marks a listed posting for liveness refresh
- **THEN** the emitted posting carries the same external id, title and URL the full path would
  have produced, so the pipeline can resolve it to the stored row

#### Scenario: Falls back to hydrating every posting without a seen-set

- **WHEN** the pipeline cannot supply a seen-set (e.g. a non-DB caller)
- **THEN** the adapter fetches every listed posting's detail, as before

### Requirement: The successfactors adapter resolves a hub board's employer from the job URL

For a `successfactors` board marked as a community hub, the adapter SHALL set each job's company
from the tenant key carried in that job's URL, resolved through the entry's `tenants` map.
SuccessFactors hub sites serve every tenant from one host and one shared `job_sitemap.xml`, and
they identify the tenant only as the FIRST path segment of a job URL
(`/<tenant>/job/<slug>/<id>/`). The job page's own `hiringOrganization` microdata SHALL NOT be
used for this: on a hub it names the corporate parent rather than the employer, and the same
markup emits the property more than once with non-company values.

A job URL whose path does not begin with a tenant segment — the tenant-less
`/job/<slug>/<id>/` shape a hub sitemap also lists — SHALL yield no tenant key, and the literal
`job` segment SHALL NOT be read as a tenant. A missing or unmapped tenant key SHALL fall back to
the configured entry company. A non-hub `successfactors` board's behaviour is unchanged (company
is the configured entry company).

#### Scenario: A mapped tenant becomes the job's employer

- **WHEN** a hub board's job URL is `https://<host>/Riverty/job/Berlin-Software-Engineer-10623/1425618633/`
  and the entry's `tenants` map has `Riverty: Riverty`
- **THEN** the job's company is `Riverty`

#### Scenario: An unmapped tenant falls back to the hub company

- **WHEN** a hub board's job URL carries the tenant segment `Sonopress` and the entry's `tenants`
  map has no `Sonopress` key
- **THEN** the job's company is the entry's configured `company`

#### Scenario: A tenant-less job URL is not attributed to a company called "job"

- **WHEN** a hub board's job URL is `https://<host>/job/Dortmund-Delphi-Entwickler-44369/1431430433/`
- **THEN** the job's company is the entry's configured `company`, never `job`

#### Scenario: A non-hub successfactors board is unaffected

- **WHEN** a `successfactors` entry has no `hub` flag
- **THEN** every job's company is the configured `company`, whatever the job URL's path segments
  are
