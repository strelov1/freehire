## ADDED Requirements

### Requirement: recrutei is a registered provider

The system SHALL register a `recrutei` adapter so a Recrutei tenant's public job board
(`jobs.recrutei.com.br/<board>`) can be crawled by board id. The board id SHALL be the
tenant's path segment. The listing SHALL be fetched once per crawl via one POST to the
tenant's `per-departments` endpoint to enumerate every open posting (id, title, company
name, location, and employment regime), and the sum of every department's item count SHALL
equal the response's declared total or the whole `Fetch` fails. Each posting's own detail
page SHALL then be fetched to complete its description and post date via the page's
schema.org ld+json block. A failure fetching one posting's detail SHALL mark only that
posting Unreadable, leaving every other posting unaffected; a listing fetch, decode, or
completeness-check failure SHALL fail the whole `Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, an employment type derived from the listing's
  Brazilian labor-regime field, the listing's own location, and the listing's company name

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing POST fails to fetch or its response cannot be decoded
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A listing whose declared total disagrees with its item count fails the whole board

- **WHEN** the response's `data.total` does not equal the sum of every department's item
  count
- **THEN** `Fetch` returns an error rather than yielding an unproven partial job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: A Brazilian labor regime maps to an employment type

- **WHEN** a listing item's `regime` is `CLT` or `Pessoa Jurídica`
- **THEN** the yielded job's employment type is `full_time` or `contract` respectively, and
  an ambiguous or unstated regime (`CLT ou PJ`, `Não informado`, or any other value) yields
  an empty employment type rather than a guess
