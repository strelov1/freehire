# micro1-source Specification

## Purpose
TBD - created by archiving change add-micro1-source. Update Purpose after archive.
## Requirements
### Requirement: micro1 job board crawl

The system SHALL provide a `micro1` source adapter that crawls the micro1 job
board (`jobs.micro1.ai`) into the catalogue. It is a **boardless single-company**
adapter: config entries carry no board, the host is fixed, and it is excluded from
the source facet. The crawl is keyless. It enumerates
`https://jobs.micro1.ai/sitemap.xml`, keeps the canonical `/post/<uuid>` posting
URLs, fetches each posting page, and maps the job payload embedded in the page's
Next.js RSC flight to a `Job`.

#### Scenario: Sitemap enumerates, each posting maps to a Job

- **WHEN** the adapter fetches its configured company entry
- **THEN** it returns one `Job` per `/post/<uuid>` URL in the sitemap, with the
  posting's role title, sanitized HTML description, location, apply/detail URL,
  posted-at date, and structured skills read from the posting page's RSC-flight
  `data` object

#### Scenario: Stable dedup identity from the client_job_id

- **WHEN** the adapter maps a posting whose `data.client_job_id` is `{uuid}`
- **THEN** the `Job.ExternalID` is `{uuid}`, so re-crawling dedups to the same
  catalogue row

#### Scenario: Description resolved from its flight reference

- **WHEN** the posting's `data.job_description` is a flight reference (e.g. `$15`)
  rather than an inline string
- **THEN** the adapter resolves the referenced chunk from the same flight stream,
  strips its length-prefix marker, and sanitizes the HTML into `Job.Description`

#### Scenario: Only canonical post URLs are crawled

- **WHEN** the sitemap contains the board root (`https://jobs.micro1.ai`) alongside
  the `/post/<uuid>` postings
- **THEN** only the `/post/<uuid>` URLs are crawled; the root and any non-post URL
  are skipped

#### Scenario: Remote work mode from the location payload

- **WHEN** a posting's `data.location_type` marks it remote (or its location name
  is empty)
- **THEN** the mapped `Job` is flagged remote with `WorkMode` set to `remote`;
  otherwise the location text is carried through for the pipeline to parse

#### Scenario: A posting page without a parseable payload is skipped

- **WHEN** a posting page fetch fails, carries no RSC-flight `data` object, or has
  no `client_job_id`
- **THEN** that posting is skipped and the rest of the crawl still returns their
  Jobs

