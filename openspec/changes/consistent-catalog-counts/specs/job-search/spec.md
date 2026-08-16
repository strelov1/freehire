## MODIFIED Requirements

### Requirement: DB-backed jobs list is index-served with an approximate total

The DB-backed `GET /api/v1/jobs` list endpoint SHALL return open jobs
(`closed_at IS NULL`) ordered newest-added first (`created_at` descending, `id`
descending) with `limit`/`offset` pagination, using the standard list envelope
`{"data": [...], "meta": {...}}`. The ordered page SHALL be served through a
partial index matching that order (no full-table sort at request time), so the
endpoint stays responsive at catalogue scale (millions of open jobs).

The `meta.total` for this endpoint SHALL be the exact open-job count from the
current catalogue-scale snapshot when one is available, and the approximate
estimate only when it is not. The endpoint SHALL NOT run a query whose cost
grows linearly with the catalogue size on each request — which is precisely why
the exact count is read from a precomputed snapshot rather than counted per
request.

Whichever figure is served, it SHALL describe the same set of postings the
endpoint paginates: open, not duplicate-suppressed, and not private. The
approximate fallback SHALL apply that full predicate, so it is an estimate of
the right set rather than an estimate of a larger one.

#### Scenario: List returns a page ordered newest-added first

- **WHEN** a client requests `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** up to 20 open jobs are returned ordered by `created_at` descending
  (ties broken by `id` descending), in the `{"data": [...], "meta": {...}}`
  envelope

#### Scenario: Meta carries the exact total when a snapshot is available

- **WHEN** a catalogue-scale snapshot is available and a client requests
  `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** `meta` reports the applied `limit` and `offset` and a `total` equal
  to the snapshot's exact open-job count

#### Scenario: Meta falls back to an approximate total when no snapshot exists

- **WHEN** no catalogue-scale snapshot is available and a client requests
  `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** `meta` reports the applied `limit` and `offset` and a `total` that is
  an approximate open-job count, and the request succeeds

#### Scenario: The approximate estimate describes the paginated set

- **WHEN** the catalogue contains postings suppressed as duplicates or marked
  private and the approximate fallback is served
- **THEN** those postings are excluded from the estimate, as they are from the
  returned page
