## ADDED Requirements

### Requirement: habr_career is a registered boardless aggregator provider

The system SHALL register a `habr_career` adapter so the Habr Career (`career.habr.com`) IT job
board can be listed in a board file as a boardless aggregator (its config entry carries no
`board`, and the provider remains in the source facet). The adapter SHALL enumerate the board
from the public, keyless listing API `GET https://career.habr.com/api/frontend/vacancies`,
sending `type=all` and `sort=date` and paginating with an increasing `page`, and SHALL send the
request headers `Accept: application/json` and `Referer: https://career.habr.com/vacancies`.
Pagination SHALL stop when the response's `meta.currentPage` reaches `meta.totalPages` (or the
`list` is empty), bounded by a maximum page count so a missing or invalid total cannot loop.
Each vacancy's employer SHALL come from the vacancy's own `company.title` (not the configured
company, which is a validation placeholder). The adapter SHALL yield the normalized job shape
with `external_id` set to the vacancy's numeric `id`, `url` set to
`https://career.habr.com/vacancies/<id>`, `title` set to `title`, `location` set to the
vacancy's distinct `locations[].title` values joined, and `posted_at` parsed from
`publishedDate.date`.

#### Scenario: The board is enumerated from the paginated public listing API

- **WHEN** a board file lists a boardless entry with provider `habr_career`
- **THEN** the adapter requests `https://career.habr.com/api/frontend/vacancies?type=all&sort=date`
  with increasing `page`, stops once `meta.currentPage` reaches `meta.totalPages`, and yields
  each listed vacancy as the normalized job shape with `external_id` set to the vacancy `id`,
  `url` set to `https://career.habr.com/vacancies/<id>`, `company` set to the vacancy's own
  `company.title`, and `posted_at` parsed from `publishedDate.date`

#### Scenario: A failed first page is a board error; a failed later page ends enumeration

- **WHEN** the first listing-API page request fails
- **THEN** the adapter returns an error for the board
- **WHEN** a later page request fails after at least one page succeeded
- **THEN** the adapter stops paginating and returns the jobs gathered so far without error

### Requirement: habr_career deduplicates with the Habr linksource adapter

The `habr_career` board adapter SHALL emit the same identity as the existing Habr Career
link-following adapter (`internal/linksource`) for the same vacancy, so a vacancy crawled from
the board and the same vacancy followed from a Telegram link resolve to one catalogue row.
Both SHALL use `source = "habr_career"`, `external_id` set to the numeric vacancy id, and `url`
set to the canonical `https://career.habr.com/vacancies/<id>`. The shared logic that parses a
Habr vacancy detail page's `JobPosting` ld+json into a description SHALL live in one helper used
by both adapters rather than being duplicated.

#### Scenario: Board-crawled and link-followed Habr vacancies dedup into one row

- **WHEN** the same Habr vacancy is both crawled by the `habr_career` board adapter and resolved
  from a Telegram link by the linksource adapter
- **THEN** both yield `source = "habr_career"`, the same numeric `external_id`, and the same
  canonical `url`, so the pipeline's upsert dedup key collapses them into a single row

### Requirement: habr_career descriptions come from the per-vacancy detail page

The listing API does not include vacancy descriptions, so the adapter SHALL fetch each
vacancy's detail page `GET https://career.habr.com/vacancies/<id>` and parse the `JobPosting`
ld+json `description`, yielding it as the job `description` after HTML sanitization (active
content stripped, structure such as paragraphs and lists kept). When the detail page has no
`JobPosting` ld+json or the detail request fails, the adapter SHALL still yield the vacancy with
the metadata available from the listing rather than dropping it.

#### Scenario: Full description is taken from the detail page and sanitized

- **WHEN** a vacancy's detail page exposes a `JobPosting` ld+json with a non-empty `description`
- **THEN** the adapter yields that HTML as the job `description`, sanitized

#### Scenario: Missing or failed detail still yields the vacancy

- **WHEN** a vacancy's detail page has no `JobPosting` ld+json or its request fails
- **THEN** the adapter yields the vacancy with its listing-derived fields and an empty
  `description` rather than dropping it

### Requirement: habr_career derives work mode and posted date from structured listing fields

The adapter SHALL mark a vacancy remote when and only when the listing item's `remoteWork` is
`true`, and SHALL set the structured work mode to `remote` in that case (leaving it empty
otherwise, so the pipeline's location parser decides on-site/hybrid). The posted date SHALL be
parsed from the listing item's `publishedDate.date` (an RFC 3339 timestamp); the adapter SHALL
NOT read the detail page's `<time class="basic-date">` element, which is the page render time
rather than the publish date.

#### Scenario: Remote flag drives the structured work mode

- **WHEN** a listing item has `remoteWork` set to `true`
- **THEN** the adapter marks the job remote and sets the structured work mode to `remote`
- **WHEN** a listing item has `remoteWork` set to `false`
- **THEN** the adapter leaves the structured work mode empty and the job not-remote, so the
  pipeline falls back to parsing the location string

#### Scenario: Posted date comes from the listing, not the detail page

- **WHEN** the adapter maps a vacancy
- **THEN** `posted_at` is parsed from the listing item's `publishedDate.date` and the detail
  page's `basic-date` time element is not used
