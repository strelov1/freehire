## ADDED Requirements

### Requirement: Experience years is filterable as a range

The search endpoints SHALL accept an `experience_years_max` parameter that bounds a
posting's stated experience requirement from above. When it is a non-negative
integer `N`, the search SHALL be restricted to documents whose
`enrichment.experience_years_min` is at most `N`. When the parameter is absent,
empty, negative, or not a valid integer, it SHALL impose no restriction. A negative
ceiling is rejected rather than honoured: the attribute is never below zero, so such
a filter can only match nothing, and serving an empty page would present a typo as a
legitimately narrow search.

The existing `experience_years_min` parameter keeps its current meaning unchanged —
it lower-bounds the same attribute (`enrichment.experience_years_min >= N`). The two
parameters SHALL compose as an AND, so supplying both expresses a closed range. The
addition is backward compatible: no existing parameter changes name or semantics.

Because `enrichment.experience_years_min` is absent on a posting whose experience
requirement was never stated, either bound SHALL exclude such postings. This
follows from Meilisearch's numeric comparison semantics and is deliberate — the
filter answers "the posting asks for at most N years", which an unstated
requirement does not satisfy.

#### Scenario: An upper bound restricts to postings asking for no more

- **WHEN** a client requests `GET /api/v1/jobs/search?experience_years_max=3`
- **THEN** only postings whose stated experience requirement is 3 years or fewer are
  returned

#### Scenario: Both bounds express a closed range

- **WHEN** a client requests
  `GET /api/v1/jobs/search?experience_years_min=2&experience_years_max=5`
- **THEN** only postings whose stated experience requirement is between 2 and 5
  years inclusive are returned

#### Scenario: An invalid upper bound imposes no restriction

- **WHEN** a client requests `GET /api/v1/jobs/search` with `experience_years_max`
  absent, empty, negative, or non-numeric
- **THEN** the result is not restricted by experience years

#### Scenario: Postings with no stated requirement fall outside a bounded range

- **WHEN** a posting carries no `enrichment.experience_years_min` and a client
  requests `experience_years_max=10`
- **THEN** that posting is not returned

#### Scenario: A zero upper bound selects the no-experience postings

- **WHEN** a client requests `GET /api/v1/jobs/search?experience_years_max=0`
- **THEN** only postings whose stated experience requirement is `0` are returned
