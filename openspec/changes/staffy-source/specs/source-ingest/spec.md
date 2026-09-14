## ADDED Requirements

### Requirement: staffy is a registered provider

The system SHALL register a `staffy` adapter as a boardless single-company source over
`jobs.wearestaffy.com`. The listing SHALL be fetched once per crawl from the platform's
static `/vacantes` page, and the count of distinct posting links found SHALL be verified
against the page's own declared total; a mismatch SHALL fail the whole `Fetch`. Each
posting's own detail page SHALL then be fetched to complete its description, location,
work mode, and seniority from its own fixed metadata fields. A failure fetching one
posting's detail SHALL mark only that posting Unreadable, leaving every other posting
unaffected; a listing fetch, decode, or completeness-check failure SHALL fail the whole
`Fetch`.

#### Scenario: The board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, a seniority level derived from the detail page's own
  label, a work mode derived from the detail page's own work-arrangement text, and the
  configured company name

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** the listing carries no postings and declares a total of zero
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing page fails to fetch or carries no parseable posting links
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A listing whose declared total disagrees with its link count fails the whole board

- **WHEN** the number of distinct posting links found does not equal the page's own
  declared total
- **THEN** `Fetch` returns an error rather than yielding an unproven partial job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: A detail page with no parseable metadata block marks only that posting unreadable

- **WHEN** a posting's detail page answers successfully but carries no metadata block at
  all
- **THEN** the adapter yields that posting as an Unreadable marker rather than a job with
  empty structured fields or a mis-bounded description

#### Scenario: Seniority maps from a closed, confirmed vocabulary

- **WHEN** a posting's detail states a seniority label of `Junior`, `Jr`, `Semi senior`,
  `Ssr`, `Senior`, `Sr`, or `Staff` (case-insensitive)
- **THEN** the yielded job's seniority is `junior`, `middle`, `senior`, or `staff`
  respectively; a compound or otherwise unrecognized label yields an empty seniority
  rather than a guess
