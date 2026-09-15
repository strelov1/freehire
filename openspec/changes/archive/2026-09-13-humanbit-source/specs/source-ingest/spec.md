## ADDED Requirements

### Requirement: humanbit is a registered provider

The system SHALL register a `humanbit` adapter so a HumanBit tenant's public job board
(`jobs.humanbit.ai/<board>`) can be crawled by board id. The board id SHALL be the tenant's
path segment. The listing page SHALL be fetched once per crawl to enumerate every open
posting (id, title, and the platform's own company display name), and each posting's own
detail page SHALL then be fetched to complete the structured fields (employment type,
remote flag, skills) the listing does not carry. Each posting's HTML description — a
`"$<id>"` reference into a flight's text rows — SHALL be resolved before being sanitized
into the job's description. A failure fetching one posting's detail SHALL mark only that
posting Unreadable, leaving every other posting unaffected; a listing fetch or decode
failure SHALL fail the whole `Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, structured employment type/remote flag/skills where
  the detail states them, and the company name the listing carries

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing page fails to fetch or its flight cannot be decoded
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: A description reference resolves to the flight's text row

- **WHEN** a posting's `description` field is a `"$<id>"` reference
- **THEN** the yielded job's description is the sanitized HTML of that id's text row, not
  the literal reference string
