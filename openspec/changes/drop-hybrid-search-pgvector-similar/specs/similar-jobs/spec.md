## MODIFIED Requirements

### Requirement: Precomputed nearest neighbours, looked up by job

The system SHALL maintain, for every open job with a persisted embedding, a
precomputed ordered list of its nearest-neighbour job IDs (`jobs.similar_job_ids`),
recomputed by a background worker rather than queried live. Recomputation SHALL
run a single pgvector nearest-neighbour query per job against
`jobs.semantic_embedding_vec` and SHALL be idempotent and incremental — a job
already carrying an up-to-date result SHALL NOT be recomputed until its own
embedding changes.

The source job SHALL never appear in its own precomputed list. The list length
SHALL be bounded by a fixed worker-side limit at least as large as the API's
maximum caller-supplied limit. A job whose embedding is not yet computed, or
whose precomputed list has not yet been populated, SHALL be treated as having no
neighbours (an empty list), not an error.

#### Scenario: Nearest neighbours are computed and stored for a job

- **WHEN** the backfill worker processes a job that has a persisted embedding but
  no precomputed neighbours yet
- **THEN** its nearest neighbours (by embedding cosine distance) are written to
  `jobs.similar_job_ids`, excluding the job itself

#### Scenario: An already-current job is not recomputed

- **WHEN** the backfill worker runs and a job's precomputed list is already
  current for its present embedding
- **THEN** that job is not reprocessed in that run

#### Scenario: A content change invalidates the precomputed list

- **WHEN** a job's embedding is recomputed because its content changed
- **THEN** its precomputed similar-jobs list is marked stale and picked up by the
  next backfill run

#### Scenario: A job with no neighbours yields an empty list

- **WHEN** the precomputed list for a job is empty (no other job was close enough
  at compute time)
- **THEN** the similar-jobs read path returns an empty list rather than an error

#### Scenario: A closed neighbour is dropped from the response, not the stored list

- **WHEN** a job referenced in another job's precomputed `similar_job_ids` has
  since closed
- **THEN** the read path omits it from the response (only still-open jobs are
  returned) without requiring the stored list itself to be corrected

### Requirement: Public similar-jobs endpoint

The system SHALL expose `GET /api/v1/jobs/:slug/similar` as a public
(unauthenticated) endpoint. It SHALL resolve `:slug` to the job's internal `id`,
read that job's precomputed `similar_job_ids`, fetch and filter to still-open
jobs, and respond with the standard list envelope `{"data": [...]}` whose `data`
is the neighbouring jobs in the public job wire shape. Each result SHALL
identify its job by `public_slug` and SHALL NOT include the internal numeric
`id`, consistent with the other public job reads. An optional `limit` query
parameter SHALL bound the number of results, clamped to a sane maximum, with a
default when absent.

Requesting similar jobs for an unknown slug SHALL return 404. The existing public
job reads SHALL be unchanged.

#### Scenario: Similar jobs for a known slug

- **WHEN** a client requests `GET /api/v1/jobs/<known-slug>/similar`
- **THEN** the response is `{"data": [...]}` listing neighbouring open jobs, each
  carrying its `public_slug` and omitting the internal numeric `id`

#### Scenario: Limit bounds the result count

- **WHEN** a client requests `GET /api/v1/jobs/<known-slug>/similar?limit=3`
- **THEN** at most 3 jobs are returned

#### Scenario: Unknown slug is a 404

- **WHEN** a client requests `GET /api/v1/jobs/<unknown-slug>/similar`
- **THEN** the response status is 404
