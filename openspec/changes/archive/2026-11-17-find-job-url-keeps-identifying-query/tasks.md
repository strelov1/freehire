## 1. Tracking vocabulary

- [x] 1.1 Export the tracking-parameter names from `internal/ingest/linksource` and add the
  well-known tracking keys; unit-test that they never veto a link match.

## 2. Normalization

- [x] 2.1 Integration tests (`internal/platform/db`): the normalized form keeps
  identifying query parameters, drops tracking ones, ignores parameter order and case, and
  still collapses the cosmetic variants; `FindOpenJobByURL` no longer answers a sibling
  posting, a listing, or a bare path with a query-identified posting.
- [x] 2.2 New no-transaction migration: replace `normalize_job_url`, then
  `REINDEX INDEX CONCURRENTLY` both expression indexes. Update the query comment.
- [x] 2.3 Integration test (`internal/api/handler`): `/jobs/find` answers the Njoyn
  posting only for its own page; SQL strips every name `linksource` lists and keeps
  identity parameters.

## 3. Verify

- [x] 3.1 `go vet -tags=integration ./...`, unit + affected integration suites, `pnpm check:sql`, EXPLAIN shows index use.
