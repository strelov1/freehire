## ADDED Requirements

### Requirement: A filter Meilisearch rejects degrades rather than failing the request

When a search request to `GET /api/v1/jobs/search` (or its agent variant) carries at least
one facet or scalar filter and the underlying search fails because Meilisearch rejected the
filter itself (as opposed to a general engine or transport failure), the system SHALL retry
the search once with the dynamic filter dropped entirely — query text, sort, vector ranking,
and pagination unchanged. On a successful retry, every query param that is part of the known
filter vocabulary and was present on the request SHALL be reported in `meta.ignored_params`,
using the same shape (and merged with) the report an unrecognized param already receives.

This SHALL NOT apply when the request carried no dynamic filter to blame, or when the
underlying failure is not classified as a filter rejection (a genuine engine or transport
failure) — such a request SHALL fail exactly as it does today.

The system SHALL NOT attempt to identify which single filter param caused the rejection.
Every active filter param SHALL be reported, since Meilisearch's own error does not reliably
name the offending attribute.

#### Scenario: A filter Meilisearch cannot yet honor degrades to an unfiltered result

- **WHEN** a search request carries a facet filter and Meilisearch rejects the query because
  that filter cannot currently be evaluated
- **THEN** the response is `200` with results as if the filter had not been sent
- **AND** `meta.ignored_params` names every filter param the request carried

#### Scenario: A request with no filter never triggers the retry

- **WHEN** a search request carries no facet or scalar filter and fails
- **THEN** the response reports the failure exactly as it does today, with no retry attempted

#### Scenario: A non-filter failure is not degraded

- **WHEN** a search request fails for a reason that is not a filter rejection (e.g. the search
  engine is unreachable)
- **THEN** the response reports the failure exactly as it does today, with no retry attempted

#### Scenario: A retry that also fails reports the original outcome

- **WHEN** a search request carries a filter, the primary search is rejected as a filter
  error, and the retry without the filter also fails
- **THEN** the response reports a failure, not a degraded success
