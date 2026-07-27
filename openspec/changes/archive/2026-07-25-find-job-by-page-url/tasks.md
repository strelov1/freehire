## 1. Normalization function and index

- [x] 1.1 Add the migration defining `normalize_job_url(text)` as an `IMMUTABLE PARALLEL
  SAFE STRICT` SQL function (lowercase; strip `http(s)://` and a leading `www.`; strip
  query and fragment; strip trailing slashes) and the partial expression index
  `jobs_normalized_url_idx ON jobs (normalize_job_url(url)) WHERE closed_at IS NULL AND
  duplicate_of IS NULL`. Comment it in the style of `0027_jobs_greenhouse_jobid_idx.sql`,
  including the note that prod applies the index as `CREATE INDEX CONCURRENTLY`.
- [x] 1.2 Integration test (build-tag `integration`, testcontainers) asserting the function
  itself: scheme, `www.`, case, query, fragment and trailing-slash variants of one URL all
  normalize to the same string; two genuinely different paths do not.

## 2. Catalog lookup by URL

- [x] 2.1 Write `FindOpenJobByURL` in `internal/db/queries/jobs.sql`: select the
  `public_slug` of the open canonical posting whose `normalize_job_url(url)` equals
  `normalize_job_url(@url)`, ordered `last_seen_at DESC, id DESC`, `LIMIT 1`. Regenerate
  sqlc (`make sqlc`).
- [x] 2.2 Integration test covering: resolution by exact URL; resolution despite a
  `?utm_source=` tail, a `www.` prefix, a scheme difference and a trailing slash; no
  resolution for a closed posting; no resolution for a `duplicate_of` posting; the most
  recently seen row wins when two open canonical rows share a URL.
- [x] 2.3 Confirm with `EXPLAIN` in the integration test's database that the query uses
  `jobs_normalized_url_idx` rather than a sequential scan.

## 3. Handler fall-through

- [x] 3.1 Extend `internal/handler/find_job.go`: when `sources.RefFromURL` reports no
  identity, or reports one that matches no row, fall through to `FindOpenJobByURL`; keep
  the `{"data": null}` shape for a miss and the identity path first for a hit.
- [x] 3.2 Handler integration test: a Greenhouse URL still resolves by identity; an
  aggregator page URL resolves by the URL fallback; an unknown page answers
  `{"data": null}`.
- [x] 3.3 Reject an empty `url` before the second tier: an empty query normalizes to `""`
  and would otherwise resolve to a posting stored with an empty `url` (found by review,
  covered by a regression test).

## 4. Verification

- [x] 4.1 Against a database seeded with a himalayas posting, call
  `/api/v1/jobs/find?url=https://himalayas.app/companies/mindera/jobs/staff-java-backend-developer?utm_source=freehire.me`
  and confirm it answers that posting's slug — the case this change exists for. (Covered
  by the handler integration test's "aggregator page resolves by its stored URL".)
- [x] 4.2 Run the repo's checks and confirm green: `go build ./...`, `go vet`, `go test
  ./...` (unit, whole repo) and `go test -tags=integration ./internal/db/
  ./internal/handler/` (525s + 384s, both ok).
