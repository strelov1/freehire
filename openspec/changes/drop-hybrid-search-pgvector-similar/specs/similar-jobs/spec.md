## MODIFIED Requirements

### Requirement: Precomputed nearest neighbours, looked up by job

The system SHALL maintain, for every open job with at least one persisted
embedding chunk, a precomputed ordered list of its nearest-neighbour job IDs
(`jobs.similar_job_ids`), recomputed by a background worker rather than queried
live. A job's description is represented by one or more embedding chunks
(`job_semantic_chunks`); a candidate job's distance to the source job SHALL be
the minimum cosine distance across every (source chunk, candidate chunk) pair —
the single nearest passage determines the match, not an average across the
whole description. Recomputation SHALL be idempotent and incremental — a job
already carrying an up-to-date result SHALL NOT be recomputed until its own
chunks change.

The source job SHALL never appear in its own precomputed list. **No candidate
from the same company as the source job SHALL appear in its precomputed list**,
regardless of how close a match it is — this holds even when excluding that
company leaves fewer than the requested number of neighbours (a short or empty
list is correct in that case, not an error). The list length SHALL be bounded by
a fixed worker-side limit at least as large as the API's maximum
caller-supplied limit. A job with no embedding chunks yet, or whose precomputed
list has not yet been populated, SHALL be treated as having no neighbours (an
empty list), not an error.

#### Scenario: Nearest neighbours are computed and stored for a job

- **WHEN** the backfill worker processes a job that has embedding chunks but no
  precomputed neighbours yet
- **THEN** its nearest neighbours (by minimum chunk-to-chunk cosine distance) are
  written to `jobs.similar_job_ids`, excluding the job itself and any job from
  the same company

#### Scenario: A same-company match is excluded even when it is the closest candidate

- **WHEN** the nearest-neighbour computation for a job finds that its
  closest-matching candidate by embedding distance belongs to the same company
  as the source job
- **THEN** that candidate is excluded from the result, and the next-closest
  candidate from a different company (if any) is used instead

#### Scenario: Excluding one company can legitimately shrink the list

- **WHEN** most or all of a job's otherwise-closest matches belong to its own
  company
- **THEN** the precomputed list is correspondingly short (or empty) rather than
  padded with same-company matches to reach the usual count

#### Scenario: An already-current job is not recomputed

- **WHEN** the backfill worker runs and a job's precomputed list is already
  current for its present chunk set
- **THEN** that job is not reprocessed in that run

#### Scenario: A content change invalidates the precomputed list

- **WHEN** a job's embedding chunks are recomputed because its content changed
- **THEN** its precomputed similar-jobs list is marked stale and picked up by the
  next backfill run

#### Scenario: A job with no neighbours yields an empty list

- **WHEN** the precomputed list for a job is empty (no other-company job was
  close enough at compute time)
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
