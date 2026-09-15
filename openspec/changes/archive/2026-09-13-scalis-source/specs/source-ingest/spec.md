## ADDED Requirements

### Requirement: scalis is a registered provider

The system SHALL register a `scalis` adapter so a Scalis tenant's public job listing
(`<board>.scalis.ai`) can be crawled by board id. The board id SHALL be the tenant
subdomain. The listing SHALL be paged via `https://<board>.scalis.ai/jobs?page=N&limit=10&sortBy=SORT_BEST_MATCH`,
walked to exhaustion (a page whose result list is empty ends the walk; a page-fetch or
decode failure at any point fails the whole `Fetch`, never a silently truncated listing).
Each page's RSC-flight payload SHALL be decoded via the existing shared primitives, and
each posting's HTML description — a `"$<id>"` reference into the flight's text rows —
SHALL be resolved before being sanitized into the job's description. The adapter SHALL
yield the normalized job shape with `external_id` set to the posting's native id and
structured `employment_type`, `work_mode`, `skills`, and salary bounds populated from the
platform's own enums/fields wherever it states them.

#### Scenario: A single-page board is crawled

- **WHEN** a tenant's listing has 10 or fewer open postings
- **THEN** every posting on the first page is yielded, and the walk stops after that page
  (its next page's result list is empty)

#### Scenario: A multi-page board is crawled to exhaustion

- **WHEN** a tenant's listing spans multiple pages
- **THEN** the adapter walks pages in order and yields the union of every page's postings

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's first listing page has an empty result list
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A failed listing page fetch fails the whole board

- **WHEN** any page's fetch or flight decode fails, including a page after the first
- **THEN** `Fetch` returns an error rather than yielding a partial job set

#### Scenario: A description reference resolves to the flight's text row

- **WHEN** a posting's `descriptionHtml` field is a `"$<id>"` reference
- **THEN** the yielded job's description is the sanitized HTML of that id's text row, not
  the literal reference string

#### Scenario: Structured employment, work mode, skills, and salary are mapped

- **WHEN** a posting states a recognized `employment`/`workplace` enum value, a skills
  list, and/or a non-null salary bound
- **THEN** the yielded job carries the mapped `employment_type`/`work_mode`, the skills
  list, and the salary bounds with their stated currency
