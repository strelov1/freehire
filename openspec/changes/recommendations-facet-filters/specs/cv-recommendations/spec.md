## MODIFIED Requirements

### Requirement: Recommendations endpoint ranks jobs by the CV vector

The system SHALL expose an authenticated endpoint `GET /api/v1/me/recommendations` that returns open jobs ranked by vector similarity between the caller's persisted CV embedding and the `jobs_semantic` index, constrained to any facet filter carried on the request. It SHALL accept the same facet query params as the search endpoint (e.g. `regions`, `work_mode`, `seniority`, `category`, `skills`, salary and freshness ranges, per-facet `_exclude`/`_mode`), translate them through the same shared filter builder, and apply the resulting filter to the vector search so that only jobs matching every facet are ranked. It SHALL use the standard list envelope (`{"data": [...], "meta": {...}}`) with each item in the shared `jobview` shape and SHALL support `limit`/`offset`. When the caller has no usable CV vector (none stored, stale, no CV) or the semantic index is unavailable, it SHALL return an empty result rather than an error.

#### Scenario: Ranked recommendations for a user with a CV vector

- **WHEN** a signed-in user with a fresh persisted CV vector requests `GET /api/v1/me/recommendations`
- **THEN** the response is a list of open jobs ordered by semantic similarity to the CV vector

#### Scenario: Facet filter constrains the ranked set

- **WHEN** a signed-in user with a fresh CV vector requests recommendations with facet params (e.g. `?work_mode=remote&seniority=senior`)
- **THEN** the response contains only open jobs that match those facets, ordered by semantic similarity to the CV vector

#### Scenario: A filter that matches nothing returns an empty feed

- **WHEN** the request carries a facet filter that no open job satisfies
- **THEN** the response is a successful empty list (no error)

#### Scenario: No CV vector returns an empty feed

- **WHEN** a signed-in user with no usable CV vector requests recommendations
- **THEN** the response is a successful empty list (no error)

#### Scenario: Requires authentication

- **WHEN** a request to `GET /api/v1/me/recommendations` carries neither a valid auth cookie nor a valid API key
- **THEN** the system responds `401`

### Requirement: The recommendations page presents the feed with empty states

The web app SHALL provide a `/my/recommendations` page, reachable from the signed-in navigation, that renders the recommendations feed for the authenticated user and offers a sidebar facet filter over it. The page SHALL reuse the shared filter UI (the same modal, summary, and mobile edge tab as `/jobs`), reflect the active filters in the URL, and re-fetch the feed from `GET /api/v1/me/recommendations` with the selected facets when they change. When there are no recommendations because the user has not uploaded a CV, the page SHALL prompt them to upload one; when the feed is empty because the current filters match no job, it SHALL show a non-error "no matches" state distinct from the upload prompt; when recommendations are otherwise empty or unavailable, it SHALL show a non-error empty state.

#### Scenario: Feed renders for a user with recommendations

- **WHEN** a signed-in user with a CV vector opens `/my/recommendations`
- **THEN** the page lists the recommended jobs alongside a sidebar filter

#### Scenario: Applying a filter narrows the feed

- **WHEN** the user selects one or more facets in the recommendations sidebar
- **THEN** the feed re-fetches and lists only the recommended jobs matching those facets

#### Scenario: A filter matching nothing shows a no-matches state

- **WHEN** the user has a CV vector but the selected filters match no open job
- **THEN** the page shows a non-error "no matching jobs" state rather than the upload prompt

#### Scenario: No CV prompts upload

- **WHEN** a signed-in user who has not uploaded a CV opens `/my/recommendations`
- **THEN** the page shows a prompt to upload a CV instead of an empty error
