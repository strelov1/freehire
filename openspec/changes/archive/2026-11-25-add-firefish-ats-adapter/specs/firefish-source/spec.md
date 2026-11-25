## Purpose

Crawls Firefish-hosted career sites (a tenant's `<tenant>.firefishsoftware.com` or its branded
careers domain) into the job catalogue via each site's public RSS document at
`/rss/adverts/latest.aspx` — one unpaginated GET per board carrying every open vacancy whole.

## ADDED Requirements

### Requirement: Board-scoped crawl

The system SHALL provide a `firefish` source adapter that crawls one Firefish-hosted career
site into the catalogue. The board id is the tenant's whole careers host (e.g.
`careers.arnoldclark.com`), the host the adapter reads the RSS document at. The adapter SHALL
return one `Job` per open vacancy the site currently lists.

#### Scenario: A board yields all its live vacancies

- **WHEN** the adapter crawls a configured Firefish board whose tenant currently has several
  live vacancies
- **THEN** it returns one `Job` per vacancy, from one listing request

#### Scenario: A site with no openings yields an empty crawl

- **WHEN** a configured board's site exists but currently lists no vacancies
- **THEN** the adapter completes the crawl with no postings rather than failing it

#### Scenario: A site that is gone reads as a gone board

- **WHEN** the feed request answers 404 or 410
- **THEN** the adapter fails the crawl with the board-gone error rather than an empty read

### Requirement: Vacancy identity and exclusions

The adapter SHALL identify a vacancy by its `ffAdvert:ReferenceNumber` and address it by the
item's `<link>`. An item missing either SHALL be dropped, and a repeated reference SHALL be
kept once.

#### Scenario: An item without identity is never returned

- **WHEN** a feed item carries no reference number or no link
- **THEN** the adapter does not return a `Job` for that item

### Requirement: Posting normalization

The adapter SHALL map, per vacancy: the reference number to the external id; the item link to
the posting URL; the title; the CDATA HTML body to a sanitized description; the stated
`LocationArea` to the location; the stated `Country` through the shared country dictionary;
the stated `JobType` to the controlled employment-type vocabulary (an unrecognized value
leaves it unset); the `PostedDate` to the posted time; and the `ClosingDate` to the
application deadline even when it lies in the future.

#### Scenario: A vacancy with full facets maps whole

- **WHEN** a feed item carries a body, a location, a country, a job type and both dates
- **THEN** the returned `Job` carries each of them in its own field, with no facet guessed
  from prose

### Requirement: Structured salary is taken whole or not at all

The adapter SHALL store a salary only when the feed states a recognised `PaymentRate` and a
positive `MinimumPayment`, taking `MaximumPayment` and `Currency` with them. A 0.00 bound, an
unrecognised rate or missing fields SHALL leave the salary fields unset; the prose
`Remuneration` SHALL never be parsed into them.

#### Scenario: Withheld pay leaves the salary unset

- **WHEN** a feed item states 0.00 bounds or a rate the adapter does not recognise
- **THEN** the returned `Job` states no salary at all rather than a bare or mis-perioded one

### Requirement: A non-feed answer is a failure

The adapter SHALL fail a crawl whose answer is not the RSS document (an HTML maintenance page,
a login page), so a reshaped site is never read as an empty board.

#### Scenario: A reshaped site fails rather than reading empty

- **WHEN** the feed URL answers 200 with markup that is not the RSS channel
- **THEN** the crawl fails with an error instead of completing with no postings
