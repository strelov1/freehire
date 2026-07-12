## 1. Word-subset match arm

- [x] 1.1 Added a third `UNION ALL` arm to the `matches` CTE in
  `SuppressAggregatorDuplicatesForCompany` (`internal/db/queries/jobs.sql`): join `agg` to `ats` on
  `string_to_array(a.ntitle,' ') <@ string_to_array(t.ntitle,' ')`, requiring the aggregator title
  to have `>= 2` words, the country gate, and the seniority gate — at least one word in
  `(ats_tokens − agg_tokens)` not in a curated seniority/qualifier marker set. sqlc regenerated.

## 2. Tests

- [x] 2.1 New integration cases (`internal/db/aggregator_subset_dedup_integration_test.go`): a
  middle-word-drop aggregator title matches its ATS twin; a one-word aggregator title does not match
  by subset; a seniority-only difference (`Software Engineer` vs `Senior Software Engineer`) is NOT
  merged; a non-seniority added word (`... Payments`) IS merged; the country gate holds. Existing
  #610/#612 exact/normalized and failover tests remain green (regression).

## 3. Verification

- [x] 3.1 Full `go test -tags=integration ./internal/db/` green (`ok`, 224s); unit + vet clean. The
  subset path suppresses the word-drop aggregator copies the equality paths missed, does not merge
  seniority variants, and leaves the exact/normalized behavior unchanged. (Prod-scale confirmation
  lands on the next reindex.)
