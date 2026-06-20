## ADDED Requirements

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
