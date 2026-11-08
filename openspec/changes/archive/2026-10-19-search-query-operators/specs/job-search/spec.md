## ADDED Requirements

### Requirement: Search query operators

The jobs search query `q` SHALL accept two operators on top of Meilisearch's own quotes
and leading-minus exclusion:

- `OR`, uppercase, a standalone whitespace-delimited token outside double quotes, SHALL
  split `q` into alternatives. A posting matching any alternative is a hit.
- `title:` at the start of an alternative SHALL restrict that whole alternative to the
  `title` field. Anywhere else it SHALL be ordinary text.

A query using neither operator SHALL be sent to the search engine exactly as before the
operators existed. Hits, paging, sort, the total and facet counts SHALL describe the
merged set of all alternatives, counting a posting matched by several alternatives once.
At most 5 alternatives SHALL be accepted. More SHALL be refused with HTTP 400 and the
message `q has more than 5 OR alternatives`.

#### Scenario: OR unions alternatives

- **WHEN** a caller searches `produktchef OR "product owner"`
- **THEN** the results include postings matching either alternative
- **AND** a posting matching both appears once and is counted once in the total

#### Scenario: title: scopes an alternative to the title

- **WHEN** a caller searches `title:"product owner" -junior`
- **THEN** a posting that mentions "product owner" only in its description is not returned
- **AND** a posting titled "Junior Product Owner" is not returned

#### Scenario: A query without operators is unchanged

- **WHEN** a caller searches `golang rust` or `golang or rust`
- **THEN** the search engine receives that query text unchanged in a single-index request

#### Scenario: Sort and paging apply to the merged set

- **WHEN** a caller searches `produktchef OR golang` sorted by `posted_at:desc` with limit 1 and offset 1
- **THEN** the response holds the second-newest posting of the merged set and the total of the merged set

#### Scenario: Facet counts describe the merged set

- **WHEN** facet counts are requested for `produktchef OR golang`
- **THEN** each posting in the merged set contributes once to its facet values

#### Scenario: Too many alternatives are refused

- **WHEN** a caller searches with six alternatives
- **THEN** the response is HTTP 400 with the message `q has more than 5 OR alternatives`
