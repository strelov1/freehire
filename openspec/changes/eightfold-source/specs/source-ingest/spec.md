## ADDED Requirements

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
