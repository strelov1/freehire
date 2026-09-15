## ADDED Requirements

### Requirement: pyjamahr is a registered provider

The system SHALL register a `pyjamahr` adapter so a PyjamaHR tenant's public job board
(`jobs.pyjamahr.com/<board>`) can be crawled by board id. The board id SHALL be the
tenant's path segment. The listing SHALL be fetched page by page via the response's own
`next` URL until it is null, and reaching a page-count safety ceiling while `next` is still
non-empty SHALL fail the whole `Fetch` rather than silently stopping. Each posting's own
detail page SHALL then be fetched to complete its description, employment type, work mode,
salary (only when the platform marks it visible), and skills. A failure fetching one
posting's detail SHALL mark only that posting Unreadable, leaving every other posting
unaffected; a listing fetch, decode, or safety-ceiling failure SHALL fail the whole
`Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, an employment type derived from the detail's job
  type, a work mode derived from the detail's workplace type or remote flag, and the
  configured company name

#### Scenario: The listing pages to exhaustion via its own next link

- **WHEN** a tenant's listing spans multiple pages
- **THEN** the adapter walks pages via the response's `next` URL and yields the union of
  every page's postings, stopping when `next` is null

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** any page's fetch or decode fails, including a page after the first
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: Reaching the page safety ceiling with more pages left fails the whole board

- **WHEN** the page walk reaches its safety ceiling while the response still names a `next`
  page
- **THEN** `Fetch` returns an error rather than yielding an unproven partial job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: Salary maps only when the platform marks it visible

- **WHEN** a posting's detail states `is_salary_visible` true with both a minimum and
  maximum bound and a recognized salary period
- **THEN** the yielded job carries those bounds with their currency and period; any other
  combination (invisible, a missing bound, or an unrecognized period) yields no salary
  fields at all
