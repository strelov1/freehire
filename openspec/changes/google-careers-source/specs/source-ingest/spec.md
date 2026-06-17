## ADDED Requirements

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
