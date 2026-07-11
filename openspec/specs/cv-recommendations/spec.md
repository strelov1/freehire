# cv-recommendations Specification

## Purpose
TBD - created by archiving change cv-recommendations. Update Purpose after archive.
## Requirements
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

### Requirement: The main jobs feed offers a CV-similarity sort mode

The standalone jobs feed SHALL offer a sort control with two modes — "Newest" (the default, newest-added first) and "Recommended" (ranked by similarity to the caller's CV) — and in CV mode SHALL rank the feed via the recommendations endpoint while keeping every facet filter in effect. The selected sort SHALL round-trip through the URL (`sort=cv`) and the standalone list's persisted filter storage, so a reload, a shared link, or a return visit restores it. The `sort=cv` value SHALL be a frontend routing signal only and SHALL NOT be sent to the keyword search endpoint. The sort control SHALL appear only on the standalone feed, not on a company-scoped embedded feed. Free-text query is not combined with CV ranking; in CV mode the free-text query does not influence ranking while facet filters still apply.

The sort control SHALL be offered only to a signed-in user (the "Recommended" mode needs a CV, so a signed-out visitor has no use for it). When a user who cannot be ranked is nonetheless in CV mode, the feed SHALL prompt the appropriate next step instead of erroring: a signed-out user who reaches CV mode via a shared `sort=cv` link SHALL be prompted to sign in, and a signed-in user with no usable CV vector SHALL be prompted to add or update their CV. Because an empty CV feed is ambiguous at the API (no CV and no-match both return an empty list), the feed SHALL distinguish the no-CV prompt from an ordinary "no matches" state by whether a facet filter is applied.

#### Scenario: Switching to CV mode ranks the feed by the CV vector

- **WHEN** a signed-in user with a usable CV vector selects the "Recommended" sort on the feed
- **THEN** the feed re-fetches from the recommendations endpoint and lists open jobs ranked by similarity to the CV, and the URL carries `sort=cv`

#### Scenario: Facet filters still narrow the CV-sorted feed

- **WHEN** a user in CV mode has one or more facet filters applied (e.g. work mode, seniority)
- **THEN** the feed lists only jobs matching those facets, ranked by CV similarity

#### Scenario: CV sort round-trips on reload

- **WHEN** the feed is loaded with `sort=cv` in the URL for a signed-in user with a usable CV vector
- **THEN** the feed starts in CV mode ranked by the CV vector rather than the newest-first default

#### Scenario: The routing signal is not sent to the search endpoint

- **WHEN** the feed is in CV mode
- **THEN** the outgoing request goes to the recommendations endpoint and carries no `sort=cv` parameter to the keyword search endpoint

#### Scenario: Default (Newest) sort browses via keyword search

- **WHEN** a user selects "Newest" (or has not chosen a sort)
- **THEN** the feed lists open jobs newest-added first via the keyword search endpoint, and the URL carries no `sort` parameter

#### Scenario: The sort control is hidden for a signed-out user

- **WHEN** a signed-out user views the standalone feed
- **THEN** no sort control is shown (the default newest feed is served)

#### Scenario: Signed-out user reaching CV mode is prompted to sign in

- **WHEN** a signed-out user opens the feed with the CV sort active (e.g. a shared `sort=cv` link)
- **THEN** the feed shows a sign-in prompt and does not call the authenticated recommendations endpoint

#### Scenario: Signed-in user without a CV is prompted to upload one

- **WHEN** a signed-in user with no usable CV vector is in CV mode with no facet filter applied
- **THEN** the feed shows a prompt to add or update their CV rather than an error or a bare empty list

#### Scenario: CV mode with a non-matching filter shows a no-matches state

- **WHEN** a user in CV mode has a usable CV vector but the applied facet filters match no open job
- **THEN** the feed shows a non-error "no matches" state distinct from the upload prompt

#### Scenario: CV sort is absent on a company-scoped feed

- **WHEN** the feed is rendered embedded and scoped to a single company
- **THEN** no CV sort control is offered

