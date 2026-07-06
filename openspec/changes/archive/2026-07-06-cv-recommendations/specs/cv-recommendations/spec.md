## ADDED Requirements

### Requirement: CV embedding is persisted in the jobs' vector space

The system SHALL compute and persist a per-user CV embedding that lives in the exact same vector space as the job embeddings, by embedding the CV text through the same Meilisearch embedder that embeds jobs and reading the resulting vector back. The persisted vector SHALL be stored with the identity of the embedder that produced it, so that a change of embedder model marks the stored vector stale and it is never compared against jobs embedded by a different model. The raw CV text SHALL NOT be persisted (only the derived vector, alongside the S3 blob résumé-storage already keeps).

#### Scenario: Upload computes and stores the CV vector

- **WHEN** a signed-in user uploads or replaces their CV and both object storage and the semantic embedder are available
- **THEN** the CV text is embedded through the same embedder as jobs, the resulting vector and the embedder identity are stored on the user, and no raw CV text is persisted

#### Scenario: A stale-model vector is not used

- **WHEN** a user's persisted CV vector was produced by a different embedder identity than the current one
- **THEN** the system treats the vector as stale (recompute on next upload) and does not rank recommendations with it

#### Scenario: Embedding unavailable degrades the upload

- **WHEN** a CV is uploaded but object storage or the embedder is unavailable
- **THEN** the CV upload/skill-extraction still succeeds and simply leaves no CV vector stored

### Requirement: Recommendations endpoint ranks jobs by the CV vector

The system SHALL expose an authenticated endpoint `GET /api/v1/me/recommendations` that returns open jobs ranked by vector similarity between the caller's persisted CV embedding and the `jobs_semantic` index. It SHALL use the standard list envelope (`{"data": [...], "meta": {...}}`) with each item in the shared `jobview` shape and SHALL support `limit`/`offset`. When the caller has no usable CV vector (none stored, stale, no CV) or the semantic index is unavailable, it SHALL return an empty result rather than an error.

#### Scenario: Ranked recommendations for a user with a CV vector

- **WHEN** a signed-in user with a fresh persisted CV vector requests `GET /api/v1/me/recommendations`
- **THEN** the response is a list of open jobs ordered by semantic similarity to the CV vector

#### Scenario: No CV vector returns an empty feed

- **WHEN** a signed-in user with no usable CV vector requests recommendations
- **THEN** the response is a successful empty list (no error)

#### Scenario: Requires authentication

- **WHEN** a request to `GET /api/v1/me/recommendations` carries neither a valid auth cookie nor a valid API key
- **THEN** the system responds `401`

### Requirement: The recommendations page presents the feed with empty states

The web app SHALL provide a `/my/recommendations` page, reachable from the signed-in navigation, that renders the recommendations feed for the authenticated user. When there are no recommendations because the user has not uploaded a CV, the page SHALL prompt them to upload one; when recommendations are simply empty or unavailable, it SHALL show a non-error empty state.

#### Scenario: Feed renders for a user with recommendations

- **WHEN** a signed-in user with a CV vector opens `/my/recommendations`
- **THEN** the page lists the recommended jobs

#### Scenario: No CV prompts upload

- **WHEN** a signed-in user who has not uploaded a CV opens `/my/recommendations`
- **THEN** the page shows a prompt to upload a CV instead of an empty error
