# ats-providers Specification

## Purpose

Hosted ATS providers registered for board crawls (Workable, Recruitee, SmartRecruiters, Personio, Pinpoint, Rippling, BambooHR, Gem, Teamtailor, JOIN, Deel). Split out of source-ingest, which holds the rules every adapter shares.

## Requirements

### Requirement: Workable, Recruitee, and SmartRecruiters are registered providers

The system SHALL register `workable`, `recruitee`, and `smartrecruiters` adapters so
boards on these platforms can be listed in a `sources/` board file. Each adapter SHALL yield the
job description as sanitized HTML assembled from the platform's authoritative HTML
field(s), consistent with the existing adapters.

#### Scenario: Workable board is crawled

- **WHEN** a `sources/` board file lists a board with provider `workable`
- **THEN** the adapter fetches that account's jobs in one request and yields each with a
  sanitized HTML description from the inline `description` field

#### Scenario: Recruitee description and requirements are combined

- **WHEN** a recruitee offer carries separate `description` and `requirements` HTML
- **THEN** the adapter yields one sanitized HTML description combining both

#### Scenario: SmartRecruiters posting gains its description from detail

- **WHEN** a smartrecruiters board is crawled
- **THEN** the adapter paginates the postings list and, per posting, fetches its detail
  and yields a sanitized HTML description assembled from `jobAd.sections`

### Requirement: Personio, Pinpoint, Rippling, and BambooHR are registered providers

The system SHALL register `personio`, `pinpoint`, `rippling`, and `bamboohr` adapters so
boards on these platforms can be listed in a `sources/` board file. Each adapter SHALL yield the
normalized job shape (at least title, url, location, remote flag, description, and the
platform's native posting id) with the `description` as sanitized HTML assembled from the
platform's authoritative HTML field(s), consistent with the existing adapters. An adapter
whose list endpoint omits the description SHALL fetch each posting's detail with bounded
concurrency rather than yield an empty body, and a single failed detail SHALL drop only
that posting rather than abort the board.

#### Scenario: Personio XML feed is crawled in one request

- **WHEN** a `sources/` board file lists a board with provider `personio`
- **THEN** the adapter fetches the board's `…jobs.personio.com/xml` feed in one request and
  yields each `<position>` with a sanitized HTML description assembled from its inline
  `jobDescriptions`, and a job URL built from the board and position id

#### Scenario: Pinpoint board carries the body inline

- **WHEN** a `pinpoint` board is crawled
- **THEN** the adapter fetches the board's `…/postings.json` in one request and yields each
  posting with a sanitized HTML description assembled from its inline body sections

#### Scenario: Rippling posting gains its description from detail

- **WHEN** a `rippling` board is crawled
- **THEN** the adapter fetches the board's job list and, per posting, fetches its detail with
  bounded concurrency to obtain the role description (excluding the company boilerplate),
  still yielding the normalized job shape

#### Scenario: BambooHR posting gains its description from detail

- **WHEN** a `bamboohr` board is crawled
- **THEN** the adapter fetches `…/careers/list` and, per posting, fetches `…/careers/{id}/detail`
  with bounded concurrency to obtain the description, still yielding the normalized job shape

#### Scenario: A failed detail request drops only that posting

- **WHEN** a detail-fetching provider's board lists several postings and one posting's detail
  request fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: A board with no open postings yields no jobs without error

- **WHEN** any of these providers' feeds returns an empty posting list for a configured board
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: Gem is a registered provider

The system SHALL register a `gem` adapter so boards on the Gem platform (jobs.gem.com)
can be listed in a `sources/` board file. The adapter SHALL treat the configured `board` value as
the Gem **vanity path** and pass it verbatim as the GraphQL `boardId`. It SHALL fetch
postings from the public GraphQL endpoint `POST https://jobs.gem.com/api/public/graphql`
using the `JobBoardList(boardId)` operation, and — because that list carries no
description — SHALL fetch each posting's body via the `ExternalJobPostingQuery(boardId,
extId)` operation with bounded concurrency. A single failed detail request SHALL drop
only that posting rather than abort the board. The adapter SHALL yield the normalized job
shape (at least title, url, location, remote flag, description, and the platform's native
posting id), with the `description` as sanitized HTML assembled from the posting's
`descriptionHtml` field, consistent with the existing adapters.

#### Scenario: Gem board is crawled list-then-detail

- **WHEN** a `sources/` board file lists a board with provider `gem` and a vanity-path `board`
- **THEN** the adapter calls `JobBoardList` with that vanity path as `boardId`, and per
  returned posting calls `ExternalJobPostingQuery` with the posting's `extId` to obtain a
  sanitized HTML description, yielding each as the normalized job shape with
  `external_id` set to the posting's `extId`

#### Scenario: Remote is taken from Gem's structured flags

- **WHEN** a Gem posting reports a location with `isRemote: true` or a `job.locationType`
  of `REMOTE`
- **THEN** the adapter yields the job with its remote flag set, without relying on a
  free-text location match

#### Scenario: Posting is dated from its first-published timestamp

- **WHEN** a Gem posting carries a `firstPublishedTsSec` Unix-seconds timestamp
- **THEN** the adapter yields the job with `posted_at` derived from that timestamp, and
  yields a nil `posted_at` when the timestamp is absent or zero

#### Scenario: A failed detail request drops only that posting

- **WHEN** a Gem board lists several postings and one posting's
  `ExternalJobPostingQuery` request fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: A board with no open postings yields no jobs without error

- **WHEN** a Gem board returns an empty `jobPostings` list
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply
  skipped

### Requirement: teamtailor is a registered provider

The system SHALL register a `teamtailor` adapter so Teamtailor career sites can be listed in
a `sources/` board file. The adapter SHALL treat the configured `board` value as the career-site host and
enumerate jobs from that site's `GET https://<board>/jobs` listing HTML, taking each job-card
anchor to a `/jobs/<id>-<slug>` path as a job (with the job's native id as the leading numeric
segment of that path). The adapter SHALL paginate the listing via `?page=N`, requesting
successive pages until a page yields no job links, so boards larger than one listing page are
fully enumerated. Because the listing carries no description, the adapter SHALL fetch each job
page and extract the posting from the page's schema.org JobPosting `application/ld+json` block,
with bounded concurrency; a single failed page fetch SHALL drop only that posting rather than
abort the board. The adapter SHALL yield the normalized job shape (at least title, url, remote
flag, description, and the platform's native posting id), with the `description` as sanitized
HTML (HTML-unescaped before sanitizing, since the `ld+json` description is double-encoded),
consistent with the existing adapters. The job `location` SHALL come from the JobPosting's
`jobLocation` address (locality and country) when present and MAY be empty otherwise.

#### Scenario: Teamtailor board is enumerated from its listing page

- **WHEN** a `sources/` board file lists a board with provider `teamtailor` and a career-site host
- **THEN** the adapter fetches `https://<host>/jobs`, and per job-card anchor fetches the job
  page, yielding each as the normalized job shape with `external_id` set to the numeric id from
  the job URL and `url` set to that job URL

#### Scenario: Title, description, and date come from the JobPosting ld+json

- **WHEN** a Teamtailor job page is fetched
- **THEN** the adapter yields the job's title from the JobPosting `title`, a sanitized HTML
  description from its HTML-unescaped `description`, `posted_at` parsed from `datePosted`, and
  `location` from the `jobLocation` address when present

#### Scenario: A multi-page board is fully enumerated

- **WHEN** a board's listing spans more than one page
- **THEN** the adapter requests successive `?page=N` pages until one yields no job links, and
  yields the jobs from every page

#### Scenario: A failed job-page fetch drops only that posting

- **WHEN** a board's listing yields several jobs and one job page's fetch fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: An empty listing yields no jobs without error

- **WHEN** a board's `/jobs` listing yields no job links
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: join is a registered provider

The system SHALL register a `join` adapter so Join.com career pages can be listed in
a `sources/` board file. The adapter SHALL treat the configured `board` value as the numeric Join
company id and enumerate jobs from the public JSON API
`GET https://join.com/api/public/companies/<board>/jobs?page=N&pageSize=<size>`, requesting
successive pages until all pages reported by the response's pagination have been read, so a
company with more jobs than one page is fully enumerated. Because the list response carries no
description, the adapter SHALL fetch each job's detail from
`GET https://join.com/api/public/jobs/<id>` with bounded concurrency; a single failed detail
request SHALL drop only that posting rather than abort the board. The adapter SHALL yield the
normalized job shape (at least title, url, remote flag, description, and the platform's native
posting id), with the `description` rendered from the job's Markdown body to sanitized HTML.
The job `location` SHALL come from the listing item's city (locality and country) when present
and MAY be empty otherwise.

#### Scenario: Join board is enumerated from the public list API

- **WHEN** a `sources/` board file lists a board with provider `join` and a numeric company id
- **THEN** the adapter requests `…/companies/<id>/jobs` and, per listed job, fetches
  `…/jobs/<job-id>`, yielding each as the normalized job shape with `external_id` set to the
  API's numeric job id and `url` set to `https://join.com/companies/<company-slug>/<idParam>`

#### Scenario: All pages of a multi-page board are enumerated

- **WHEN** a board's job count spans more than one API page
- **THEN** the adapter requests each page up to the pagination's reported page count and yields
  the jobs from every page

#### Scenario: Description is rendered from Markdown to sanitized HTML

- **WHEN** a Join job's detail is fetched and its body is Markdown
- **THEN** the adapter yields a `description` that is the Markdown rendered to HTML and then
  sanitized (active content stripped, structure such as lists and paragraphs kept)

#### Scenario: A failed detail request drops only that posting

- **WHEN** a board lists several jobs and one job's detail request fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: An empty board yields no jobs without error

- **WHEN** a board's list API reports zero jobs
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: deel is a registered provider

The system SHALL register a `deel` adapter so Deel ATS career pages
(`jobs.deel.com/<orgSlug>`) can be listed in a board file. The adapter SHALL treat the
configured `board` value as the org's URL slug and enumerate that org's postings from a single
request, `GET https://jobs.deel.com/<board>`, by parsing the Next.js flight payload embedded in
the page (the concatenated, JS-string-decoded bodies of the page's `self.__next_f.push` chunks).
The adapter SHALL read the postings from the payload's `jobPostings` array and SHALL resolve
each posting's `richtextDescription` `"$N"` reference to the corresponding text row
(`N:T<byte-length>,<html>`) in the same payload, yielding it as the job `description` after HTML
sanitization. The adapter SHALL NOT issue a per-posting detail request. The adapter SHALL yield
the normalized job shape (at least title, url, description, and the platform's native posting
id), with `external_id` set to the posting `id` (the UUID used in the public job URL) and `url`
set to `https://jobs.deel.com/<board>/job-details/<id>/overview`. The job `company` SHALL come
from the payload's `careerPageSettings.preferredOrganizationName` when present, falling back to
the configured company name. The job `location` SHALL come from the posting's job-location names
when present and MAY be empty otherwise. Because Deel exposes no structured workplace-type field,
the adapter SHALL leave the structured work mode empty and infer the remote flag from the shared
location heuristic.

#### Scenario: Deel board is enumerated from the embedded flight payload

- **WHEN** a board file lists an entry with provider `deel` and an org URL slug as the board
- **THEN** the adapter requests `https://jobs.deel.com/<slug>` once and yields each posting from
  the page's `jobPostings` payload as the normalized job shape, with `external_id` set to the
  posting `id` and `url` set to `https://jobs.deel.com/<slug>/job-details/<id>/overview`, issuing
  no per-posting detail request

#### Scenario: Description is resolved from a `$N` reference and sanitized

- **WHEN** a posting's `richtextDescription` is a `"$N"` reference to a `N:T<byte-length>,<html>`
  text row in the same payload
- **THEN** the adapter resolves the reference to that row's HTML (sliced by its declared byte
  length) and yields it as the job `description`, sanitized (active content stripped, structure
  such as paragraphs and lists kept) and decoded as correct UTF-8

#### Scenario: Company name comes from the career-page settings

- **WHEN** the payload's `careerPageSettings.preferredOrganizationName` is present
- **THEN** the yielded jobs use it as the company name; when it is absent the adapter falls back
  to the configured company name

#### Scenario: An empty or jobless board yields no jobs without error

- **WHEN** a board's page carries an empty `jobPostings` payload
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

#### Scenario: A malformed payload is an error, not silent bad data

- **WHEN** the page does not contain a decodable `jobPostings` payload
- **THEN** the adapter returns an error for that board rather than yielding zero jobs silently,
  so a markup change surfaces instead of quietly emptying the catalogue
