## MODIFIED Requirements

### Requirement: CV embedding is persisted in the jobs' vector space

The system SHALL compute and persist a per-user CV embedding that lives in the
exact same vector space as the job embeddings, by embedding the CV text through
the same embedding backend (TEI) that embeds jobs and storing the resulting
vector. The persisted vector SHALL be stored with the identity of the embedder
that produced it, so that a change of embedder model marks the stored vector
stale and it is never compared against jobs embedded by a different model. The
raw CV text SHALL NOT be persisted (only the derived vector, alongside the S3
blob résumé-storage already keeps).

#### Scenario: Upload computes and stores the CV vector

- **WHEN** a signed-in user uploads or replaces their CV and both object storage
  and the embedding backend are available
- **THEN** the CV text is embedded through the same backend as jobs, the
  resulting vector and the embedder identity are stored on the user, and no raw
  CV text is persisted

#### Scenario: A stale-model vector is not used

- **WHEN** a user's persisted CV vector was produced by a different embedder
  identity than the current one
- **THEN** the system treats the vector as stale (recompute on next upload) and
  does not rank recommendations with it

#### Scenario: Embedding unavailable degrades the upload

- **WHEN** a CV is uploaded but object storage or the embedding backend is
  unavailable
- **THEN** the CV upload/skill-extraction still succeeds and simply leaves no CV
  vector stored

### Requirement: Recommendations endpoint ranks jobs by the CV vector

The system SHALL expose an authenticated endpoint `GET /api/v1/me/recommendations`
that returns open jobs ranked by pgvector cosine-distance similarity between the
caller's persisted CV embedding and each candidate job's embedding chunks
(`job_semantic_chunks`) — a job's rank uses the minimum distance across its own
chunks (the single nearest passage to the CV, same rollup rule as `/similar`'s
job-to-job matching), not an average across the whole description. It is
constrained to any facet filter carried on the request. It SHALL accept the same facet query
params as the search endpoint (e.g. `regions`, `work_mode`, `seniority`,
`category`, `skills`, salary and freshness ranges, per-facet `_exclude`/`_mode`),
translate them through the same shared filter builder used by keyword search
(`search.FilterFromValues`) against the live keyword/facet `jobs` index — not a
separate SQL translation of the same params — to obtain the set of matching job
IDs, and restrict the pgvector ranking query to that set so that only jobs
matching every facet are ranked. The candidate set from this filter step MAY be
capped at a bounded size for a very broad filter, rather than exhaustive — a
recommendations feed does not need to rank literally every possible match. It
SHALL use the standard list envelope
(`{"data": [...], "meta": {...}}`) with each item in the shared `jobview` shape
and SHALL support `limit`/`offset`. When the caller has no usable CV vector
(none stored, stale, no CV), it SHALL return an empty result rather than an
error.

#### Scenario: Ranked recommendations for a user with a CV vector

- **WHEN** a signed-in user with a fresh persisted CV vector requests
  `GET /api/v1/me/recommendations`
- **THEN** the response is a list of open jobs ordered by semantic similarity to
  the CV vector

#### Scenario: Facet filter constrains the ranked set

- **WHEN** a signed-in user with a fresh CV vector requests recommendations with
  facet params (e.g. `?work_mode=remote&seniority=senior`)
- **THEN** the response contains only open jobs that match those facets, ordered
  by semantic similarity to the CV vector

#### Scenario: A very broad filter still returns a ranked, non-error result

- **WHEN** a signed-in user with a fresh CV vector requests recommendations with
  a facet filter broad enough to match far more open jobs than the candidate-set
  cap
- **THEN** the response is a successful ranked list drawn from a bounded
  candidate set, not an error and not a requirement to consider every matching
  job

#### Scenario: A filter that matches nothing returns an empty feed

- **WHEN** the request carries a facet filter that no open job satisfies
- **THEN** the response is a successful empty list (no error)

#### Scenario: No CV vector returns an empty feed

- **WHEN** a signed-in user with no usable CV vector requests recommendations
- **THEN** the response is a successful empty list (no error)

#### Scenario: Requires authentication

- **WHEN** a request to `GET /api/v1/me/recommendations` carries neither a valid
  auth cookie nor a valid API key
- **THEN** the system responds `401`
