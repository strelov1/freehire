## ADDED Requirements

### Requirement: recruiterflow is a registered provider

The system SHALL register a `recruiterflow` adapter so a RecruiterFlow agency board's
public listing (`recruiterflow.com/<board>/jobs`) can be crawled by board id. The board id
SHALL be the tenant's path segment. The listing SHALL be read once per crawl from the
page's own embedded `window.jobsList` JavaScript object (department-grouped, no separate
network call), which SHALL name every open posting's id, title, location, employment type,
remote type, and last-opened date. Each posting's own detail page SHALL then be fetched
for its description via the page's schema.org ld+json block. A failure fetching one
posting's detail SHALL mark only that posting Unreadable, leaving every other posting
unaffected; a listing fetch or decode failure SHALL fail the whole `Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the embedded listing names is fetched for its detail and yielded
  with a title, sanitized HTML description, an employment type and work mode derived from
  the listing's own fields, the listing's own location, and the configured company name

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's embedded listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing page fails to fetch or its embedded `window.jobsList` cannot be
  decoded
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: Employment type and remote type map from the listing's own vocabulary

- **WHEN** a listing item's `employment_type` is `Full time`, `Part time`, or `Contract`,
  and its `remote_type` is `Remote`, `Hybrid`, or absent
- **THEN** the yielded job's employment type and work mode reflect those values, with an
  absent remote type yielding an empty work mode rather than a guess
