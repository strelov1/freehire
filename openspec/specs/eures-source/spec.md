# eures-source Specification

## Purpose

Crawls one EURES-covered country's job vacancies through the EURES public search and detail
APIs, scoped to ICT occupations, and marks itself as an aggregator so cross-source dedup can
suppress a copy that a more authoritative source (a national employment agency adapter, a
first-party ATS) already carries.

## Requirements

### Requirement: Country-board ICT search

The system SHALL provide an `eures` source adapter that queries the EURES search API
(`POST https://europa.eu/eures/api/jv-searchengine/public/jv-search/search`) filtered to the
board's country code (`locationCodes`) and to the ICT occupation groups (ESCO/ISCO URIs for "ICT
professionals" and "ICT technicians"). The adapter SHALL paginate the search until a page returns
fewer results than requested, the reported total is reached, or a fixed page-count cap tied to the
API's pagination depth limit is reached — whichever comes first — and SHALL NOT issue a request
beyond that cap.

#### Scenario: A country board is paginated within the depth cap

- **WHEN** the adapter crawls a board whose `board` is an EURES-covered country code
- **THEN** it requests the search API with that country in `locationCodes` and the ICT
  occupation-group filter, advancing pages until the results are exhausted or the depth-cap
  backstop is reached, without ever issuing a request past that cap

#### Scenario: Board with no matching postings

- **WHEN** the search API reports zero results for a board's country
- **THEN** the adapter returns an empty result, not an error

### Requirement: Bounded recency window

The system SHALL restrict every search request to a fixed recent-publication window sorted by
most-recent-first, chosen so that the largest observed country/occupation-group combination stays
within the API's pagination depth cap.

#### Scenario: A high-volume country stays within the depth cap

- **WHEN** the adapter crawls a board whose total matching postings would exceed the pagination
  depth cap without a recency restriction
- **THEN** the recency window keeps the requested result count within the cap, and any postings
  older than the window are left for a subsequent crawl to pick up

### Requirement: ICT-scoped technical hint

Every `Job` the adapter yields SHALL carry `IsTechHint = true`, since the adapter's fixed
occupation-group filter guarantees every result is a technical posting.

#### Scenario: Every yielded job is hinted technical

- **WHEN** the adapter yields a `Job` for any posting the ICT-scoped search returns
- **THEN** that `Job` has `IsTechHint` set to true

### Requirement: Posting normalization from search plus location detail

The adapter SHALL map each search result onto the catalogue's `Job` shape using the search
result's own title, employer name, and HTML description (no separate fetch needed for these), and
SHALL additionally fetch that posting's detail endpoint
(`GET https://europa.eu/eures/api/jv-searchengine/public/jv/id/{id}`) solely to resolve a
human-readable `Location` from the detail's structured place data (city/address), since the
search result's own location data carries only opaque region codes. A posting whose detail fetch
fails or carries no usable place data SHALL still be yielded, falling back to a location built
from the board's own country rather than being dropped.

#### Scenario: A posting maps to a Job with an enriched location

- **WHEN** a search result carries a title, employer name, description, and identifier, and its
  detail fetch succeeds with a resolvable city or address
- **THEN** the resulting `Job` carries the search result's title, employer name, and sanitized
  description, and a `Location` built from the detail's place data

#### Scenario: A failed detail fetch does not drop the posting

- **WHEN** a posting's detail fetch fails or returns no usable place data
- **THEN** the adapter still yields a `Job` for that posting, with `Location` falling back to the
  board's country

### Requirement: Structured country and employment-type signals

The adapter SHALL set `Job.Countries` from the search result's location data, normalized to
freehire's canonical country codes, and SHALL set `Job.EmploymentType` when the search result's
own offering or schedule code maps unambiguously onto freehire's employment-type vocabulary,
leaving it unset otherwise.

#### Scenario: Countries is set from structured location data

- **WHEN** a search result names one or more countries in its structured location data
- **THEN** the resulting `Job.Countries` carries those countries normalized to freehire's
  canonical codes

#### Scenario: An unambiguous offering/schedule code sets EmploymentType

- **WHEN** a search result's offering code or schedule code maps unambiguously onto freehire's
  employment-type vocabulary
- **THEN** the resulting `Job.EmploymentType` carries that mapped value

### Requirement: Canonical portal URL, not scraped application text

The adapter SHALL set `Job.URL` to the EURES portal's own detail page for the posting's
identifier, and SHALL NOT attempt to parse an application URL out of the posting's free-text
application instructions.

#### Scenario: URL is the portal detail page

- **WHEN** the adapter yields a `Job` for a posting with identifier `id`
- **THEN** `Job.URL` is the EURES portal detail page addressed by `id`, regardless of what the
  posting's own application instructions text contains

### Requirement: Aggregator marker for cross-source dedup

The adapter SHALL carry the aggregator marker, so the catalogue's cross-source dedup pass may
suppress an EURES copy of a posting that a more authoritative source already carries. It SHALL
carry neither the boardless nor the full-catalogue marker, since a board is one country's
crawl, not the whole EURES catalogue.

#### Scenario: EURES is included in the aggregator provider set

- **WHEN** the catalogue's cross-source dedup pass asks which registered providers are
  aggregators
- **THEN** `eures` is included in that set
