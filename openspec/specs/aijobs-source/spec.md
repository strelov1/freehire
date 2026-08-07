# aijobs-source Specification

## Purpose
The aijobs.net adapter: a large AI/ML job aggregator (~47k postings) crawled as a single
boardless, multi-company feed behind a Django CSRF-protected listing endpoint. The capability
covers the CSRF session flow, the seen-based pagination stop plus per-run detail-fetch budget
that bound crawling a catalogue far bigger than any single run should hydrate, and the
field-mapping rules a paywalled, structured-only detail page forces (company from a URL slug,
description from the site's own pre-processed sections, no salary).

## Requirements

### Requirement: aijobs.net is crawled as a boardless aggregator

The system SHALL crawl aijobs.net's job listing as a single global feed under provider key
`aijobs`, registered boardless (no per-tenant board id) and marked an aggregator (many
employers per crawl, employer taken from each posting rather than the configured entry).
`sources/aijobs.yml` SHALL carry exactly one placeholder entry naming the provider, mirroring
`sources/arbeitnow.yml`'s convention.

#### Scenario: A single board entry crawls the whole feed

- **WHEN** `sources/aijobs.yml` names provider `aijobs` with no board
- **THEN** config validation accepts the entry, and the crawl fetches postings from the shared
  global listing rather than a per-company scope

#### Scenario: Board omission does not fail validation

- **WHEN** a `sources/aijobs.yml` entry for provider `aijobs` carries no `board` field
- **THEN** config validation succeeds (the provider is registered boardless)

### Requirement: The listing session is authenticated per run via a CSRF cookie

The system SHALL bootstrap one HTTP session per crawl run with a `GET` to the aijobs.net home
page to obtain the `csrftoken` cookie, then issue every subsequent paginated listing request as
a `POST` carrying that same token value as both the `x-csrftoken` header and the
`csrfmiddlewaretoken` form field, and a `Referer` header naming aijobs.net. A listing request
missing the `Referer` header SHALL be treated as a failed request (the site rejects it with a
CSRF error), not retried without it.

#### Scenario: Session bootstrap precedes pagination

- **WHEN** a crawl run starts
- **THEN** the adapter first performs a `GET` to acquire the session cookie before issuing any
  paginated listing `POST`

#### Scenario: Every listing POST carries the token and Referer

- **WHEN** the adapter requests listing page N
- **THEN** the request is a `POST` whose `x-csrftoken` header, `csrfmiddlewaretoken` form
  field, and session cookie all carry the same token value, and whose `Referer` header names
  aijobs.net

### Requirement: Detail fetches happen only for postings not already in the catalogue

The system SHALL implement the hydrating fetch path (`FetchNew`), issuing a per-posting detail
`GET` only for a posting whose external id the supplied `seen` predicate reports as not yet
ingested. The plain `Fetch` path (used only when no `seen` predicate is available) has no
list-only shape to fall back to — the listing carries no company, and a posting with no company
is dropped (see "Company display name is derived from the company profile URL slug") — so
`Fetch` delegates to `FetchNew` with a predicate that reports every posting as unseen,
hydrating everything the listing yields up to the same per-run budget as a real crawl.

#### Scenario: An already-ingested posting is not re-fetched

- **WHEN** the listing yields a posting whose external id `seen` reports as true
- **THEN** the adapter does not issue a detail-page request for that posting

#### Scenario: A new posting is hydrated

- **WHEN** the listing yields a posting whose external id `seen` reports as false
- **THEN** the adapter issues a detail-page request and the returned job carries the
  detail-page fields (company, description, skills)

### Requirement: Listing pagination is bounded by a seen-page stop and a hard page cap

The system SHALL stop walking listing pages when a page's postings are ALL already reported
seen by the `seen` predicate (the newest-first feed has been caught up to), or when a fixed
per-run page count is reached, whichever comes first. The page count cap SHALL exist
independently of the seen-based stop as a safety backstop.

#### Scenario: Pagination stops once caught up to known postings

- **WHEN** every posting on a listing page is already reported seen
- **THEN** the adapter stops requesting further pages for this run

#### Scenario: Pagination stops at the hard cap regardless of seen state

- **WHEN** the per-run page cap is reached before any page is fully seen
- **THEN** the adapter stops requesting further pages for this run

### Requirement: New-posting detail fetches are bounded per run

The system SHALL cap the number of unseen postings hydrated with a detail fetch in one run to
the value of the `AIJOBS_MAX_NEW_PER_RUN` environment variable (default 500 when unset). Once
the cap is reached, the run SHALL stop issuing further detail requests and SHALL stop
discovering further listing pages for that run; postings past the cap remain unseen and are
picked up on a subsequent run.

#### Scenario: A run stops after reaching the new-posting cap

- **WHEN** the number of unseen postings queued for detail fetch in the current run reaches
  `AIJOBS_MAX_NEW_PER_RUN`
- **THEN** the adapter stops fetching further detail pages and stops requesting further
  listing pages for that run

#### Scenario: Postings past the cap are picked up next run

- **WHEN** a posting was discovered but not hydrated because the run's cap was already reached
- **THEN** that posting's external id remains unseen, and it is discovered and hydrated on a
  later run

### Requirement: Company display name is derived from the company profile URL slug

The system SHALL derive `Job.Company` from the `/company/<slug>-<id>/` link on the detail
page by stripping the trailing numeric id and title-casing the hyphen-separated slug, because
the page's own visible company name is rendered masked (paywalled). A detail page carrying no
company profile link SHALL be skipped rather than stored with an empty or masked company name.

#### Scenario: A company slug is title-cased into a display name

- **WHEN** a detail page's company link is `/company/medison-pharma-16767/`
- **THEN** `Job.Company` is set to `Medison Pharma`

#### Scenario: A detail page without a company link is dropped

- **WHEN** a detail page carries no `/company/<slug>-<id>/` link
- **THEN** that posting is not returned as a job

### Requirement: Description and skills are built from the page's structured sections, salary is dropped

The system SHALL build `Job.Description` from the detail page's "Tasks" section list items (a
newline-separated bullet rendering), and `Job.Skills` from the "Skills/Tech-stack" section's
anchor texts. A "Perks/Benefits" section whose only item is the literal text `N/A` SHALL be
omitted from the description rather than rendered; a Perks/Benefits section with real content
SHALL be appended to the description instead. The system SHALL NOT parse or store the page's
salary figure, since it is aggregator-computed ("estimate") rather than employer-stated.

#### Scenario: Tasks become the description

- **WHEN** a detail page's Tasks section lists several bullet items
- **THEN** `Job.Description` renders those items as a bullet list

#### Scenario: Skills populate the Skills field, not the description

- **WHEN** a detail page's Skills/Tech-stack section links several skill names
- **THEN** those names populate `Job.Skills` and are not duplicated into `Job.Description`

#### Scenario: A placeholder Perks section is omitted

- **WHEN** a detail page's Perks/Benefits section contains only the item `N/A`
- **THEN** no perks content is added to `Job.Description`

#### Scenario: Salary is never stored

- **WHEN** a detail page displays a salary badge marked "(estimate)"
- **THEN** the returned job carries no salary value derived from that badge

### Requirement: Posted time is parsed from the relative-time string

The system SHALL parse `Job.PostedAt` from the detail page's relative-time text, which appears
under either of two labels observed live — "Found X<unit> ago" (a recently-crawled posting) or
"Published X<unit> ago" (an older one with a recorded original date) — relative to the crawl
time, handling `h`/`d`/`w`/`mo`/`y` units (`mo`/`y` via a flat 30/365-day approximation, not
calendar-aware). A posting whose relative-time text does not match a recognized label or unit
SHALL be returned with `PostedAt` unset rather than failing the whole posting.

#### Scenario: Hours-ago is parsed

- **WHEN** a detail page reads "Found 8h ago"
- **THEN** `Job.PostedAt` is set to approximately 8 hours before the crawl time

#### Scenario: Days-ago is parsed, under either label

- **WHEN** a detail page reads "Found 3d ago" or "Published 3d ago"
- **THEN** `Job.PostedAt` is set to approximately 3 days before the crawl time in both cases

#### Scenario: An unrecognized unit leaves PostedAt unset

- **WHEN** a detail page's relative-time text uses a unit the parser does not recognize
- **THEN** the posting is still returned as a job, with `PostedAt` left nil
