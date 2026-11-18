# russian-source-providers Specification

## Purpose

The Russian single-company and aggregator providers (bigtech careers sites, avito, huntflow hubs, habr_career) and how each maps postings into the catalogue. Split out of source-ingest, which holds the rules every adapter shares.

## Requirements

### Requirement: Russian bigtech single-company providers are registered

The system SHALL register adapters for the single-company Russian career APIs `yandex`,
`ozon`, `rwb`, `sber`, `alfabank`, `lamoda`, `kuper`, `aviasales`, `dodo`, `domclick`,
`mtslink`, `tbank`, `mts`, and `vk`, so each can be listed in the boards configuration.
Each adapter SHALL yield the normalized job shape (at least title, url, location, remote
flag, description, and the platform's native posting id) with the `description` as
sanitized HTML (or sanitized text for an API that publishes plain text) assembled from the
platform's authoritative field(s). An adapter whose list endpoint omits the description
SHALL fetch each posting's detail with bounded concurrency rather than yield an empty body,
and a single failed detail SHALL drop only that posting rather than abort the board. All
providers except `yandex` SHALL be `boardless`; `yandex` SHALL select host and language
(`ru`/`com`) from its `board`.

Where a provider's public vacancy page is addressed by a different identifier than its
dedup key, the adapter SHALL build the job `url` from the page identifier while keeping the
dedup `external_id` on the provider's stable posting id.

#### Scenario: Cursor-paginated board is fully crawled

- **WHEN** a `yandex` board is crawled and its list endpoint paginates by cursor
- **THEN** the adapter follows the cursor until exhausted, skips postings that redirect out
  or are hiring events, fetches each remaining posting's detail for the description, and
  yields each as the normalized job shape with `external_id` set to the posting's native id

#### Scenario: Page-paginated board is fully crawled

- **WHEN** an `ozon` board is crawled and its list endpoint paginates by page number
- **THEN** the adapter walks every page to the reported total, keeps only externally-listed
  vacancies, fetches each posting's detail for the description, and yields each as the
  normalized job shape

#### Scenario: Offset-paginated board with inline body needs no detail call

- **WHEN** a `sber` or `alfabank` board is crawled and the list endpoint carries the full
  body inline
- **THEN** the adapter walks the offset window to the reported total and yields each posting
  directly with a sanitized description, issuing no per-posting detail request

#### Scenario: Sber posting URL is built from the numeric internalId

- **WHEN** a `sber` posting is normalized and its feed record carries both a `requisitionId`
  GUID and a numeric `internalId`
- **THEN** the job `url` is `https://rabota.sber.ru/search/<internalId>` (the identifier the
  public vacancy page resolves — the `requisitionId` GUID route returns 404)
- **AND** the job `external_id` remains the `requisitionId`, so the dedup key is unchanged

#### Scenario: Header-gated board is crawled

- **WHEN** an `mts` board is crawled and its API requires a non-secret `x-api-key` header
- **THEN** the adapter obtains the public key, sends it on each request, walks the offset
  window to the reported total, fetches each posting's detail, and yields each as the
  normalized job shape

#### Scenario: A board with no open postings yields no jobs without error

- **WHEN** any of these providers' endpoints returns an empty posting list for a configured
  board
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: avito is a registered single-company boardless provider

The system SHALL register an `avito` adapter so Avito's career site (`career.avito.com`) can be
listed in a board file as a single-company boardless source (its config entry carries no `board`,
and the provider remains in the source facet). The adapter SHALL enumerate every vacancy from the
public, keyless sitemap index `https://career.avito.com/sitemap.xml`: it SHALL fetch the index,
fetch each referenced sub-sitemap, and keep every `<loc>` that matches a vacancy-detail URL
(`/vacancies/<category>/<id>/`, a numeric trailing id), deduplicating by that numeric id. Each
vacancy's posting SHALL come from its own detail-page fetch, fanned out under the shared
bounded-concurrency detail pool. The adapter SHALL yield the normalized job shape with
`external_id` set to the numeric vacancy id from the URL, `url` set to the vacancy page URL,
`company` set to the configured company, `title`/`description`/`posted_at` parsed from the page's
`JobPosting` ld+json, and `description` HTML-sanitized.

#### Scenario: The board is enumerated from the sitemap index and vacancy pattern

- **WHEN** a board file lists a boardless entry with provider `avito`
- **THEN** the adapter fetches `https://career.avito.com/sitemap.xml`, fetches each sub-sitemap,
  keeps only `<loc>` values matching `/vacancies/<category>/<id>/`, deduplicates by the numeric id,
  and yields each vacancy as the normalized job shape with `external_id` set to that id and `url`
  set to the vacancy page URL

#### Scenario: A loc without a parseable vacancy id is skipped

- **WHEN** a sitemap `<loc>` is a category, team, or landing page with no trailing numeric vacancy
  id
- **THEN** the adapter does not treat it as a vacancy and does not fetch it as a detail page

#### Scenario: A failed sitemap fetch is a board error; a failed detail skips just that vacancy

- **WHEN** the sitemap-index request fails
- **THEN** the adapter returns an error for the board
- **WHEN** a single vacancy's detail-page request fails or the page carries no `JobPosting` ld+json
- **THEN** the adapter skips that one vacancy and still yields the rest

### Requirement: avito derives location and remote from the page title, not the JSON-LD city

The adapter SHALL derive the display location from the page `<title>` suffix (`… в городе <city>`),
falling back to the JSON-LD `addressLocality` when that suffix is absent, because Avito's
`JobPosting.jobLocation.addressLocality` reports the company HQ city (`Москва`) even for remote
roles. The adapter SHALL mark a vacancy remote when the derived location or the title matches a
remote signal (the existing `isRemote` check, which matches the Russian stem "удал"), so "Удалённая
работа" roles are classified remote rather than as on-site Moscow roles.

#### Scenario: A remote role is classified remote despite a Moscow JSON-LD city

- **WHEN** a vacancy page's `<title>` ends with "в городе Удалённая работа" while its `JobPosting`
  ld+json reports `addressLocality` "Москва"
- **THEN** the adapter sets the job location to "Удалённая работа" and marks the job remote

#### Scenario: An on-site role keeps its city

- **WHEN** a vacancy page's `<title>` ends with "в городе Москва"
- **THEN** the adapter sets the job location to "Москва" and does not mark the job remote

### Requirement: The huntflow adapter resolves a hub board's employer from the vacancy division

For a `huntflow` board marked as a community hub, the adapter SHALL set each vacancy's company
from the vacancy's `division` breadcrumb. Huntflow encodes `division` leaf-first as
`<sub-team> · … · <Company> · Partners · Vacancies`, where the literal `Partners` folder
contains every partner company; the employer is therefore the segment immediately preceding the
`Partners` segment. The adapter SHALL accept either separator Huntflow emits (`·` U+00B7 in the
list payload, `•` U+2022 in the detail payload). When the division has no `Partners` segment, or
no segment precedes it, or the resolved segment is empty, the adapter SHALL fall back to the
configured entry company. A non-hub `huntflow` board's behaviour is unchanged (company is the
configured entry company).

#### Scenario: The employer is the segment before the Partners folder

- **WHEN** a hub vacancy's division is `Mirai · Partners · Vacancies`
- **THEN** the job's company is `Mirai`

#### Scenario: A deeper sub-team does not displace the employer

- **WHEN** a hub vacancy's division is `Remote · Sparkland · Partners · Vacancies`
- **THEN** the job's company is `Sparkland` (the segment before `Partners`), not the deeper
  `Remote` sub-team

#### Scenario: Either separator is accepted

- **WHEN** a hub vacancy's division uses the bullet separator, e.g. `Fluently • Partners • Vacancies`
- **THEN** the job's company is `Fluently`

#### Scenario: A division without a partner structure falls back to the hub company

- **WHEN** a hub vacancy's division is empty or lacks a `Partners` segment
- **THEN** the job's company is the configured entry company (the hub's own name)

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
link-following adapter (`internal/ingest/linksource`) for the same vacancy, so a vacancy crawled from
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
