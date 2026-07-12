## 1. Normalized-key match

- [x] 1.1 Extend `SuppressAggregatorDuplicatesForCompany` in `internal/db/queries/jobs.sql`:
  compute a second key `ntitle2` on both the `ats` and `agg` CTEs — remove HTML entities
  (`&…;`), strip one trailing ` - `/` | `/` — ` segment when a non-empty base remains, then
  the existing lowercase-and-collapse — and add it as a second match path. Implemented as a
  UNION of two single-equality hash joins (ntitle and ntitle2), LEFT-joined back to `agg`,
  so it stays O(agg+ats) and preserves failover. Regenerate sqlc (`make sqlc`).

## 2. Tests

- [x] 2.1 New integration cases (`internal/db/aggregator_fuzzy_dedup_integration_test.go`):
  ATS `... - Leisure` suffix matches the bare aggregator title; `F&amp;B` vs `F&B` matches;
  a distinct base is not merged by the suffix-strip; the country gate still holds for the
  normalized key. Existing slice-1 exact-key and failover tests remain green (regression).

## 3. Verification

- [x] 3.1 Full `go test -tags=integration ./internal/db/` green: the normalized key
  suppresses the suffix/entity-mangled aggregator copies the exact key missed, the exact-key
  cases and failover are unchanged. (Prod-scale confirmation lands on the next reindex.)
