## 1. The closure query

- [x] 1.1 Write the whole-catalogue closure query `DuplicateClosureGeoAll` in
      `internal/platform/db/queries/jobs.sql`: recursive, seeded from open rows with
      `duplicate_of IS NULL` that own at least one open duplicate, returning
      `(owner_id, countries, regions, cities)` unioned over the closure's open members. Document
      in the comment why cycles are unreachable (the seed is nobody's duplicate) and what the
      depth bound is for. `make sqlc`.
- [x] 1.2 Add the by-id-set variant `DuplicateClosureGeoFor(owner_ids)` sharing the same recursive
      body, for the drain wave and the single-row link import. RESOLVED: no separate single-row
      query — link import passes a one-element slice, so the `:one` "always answers" contract the
      old `RoleClusterGeo` needed is replaced by an empty result meaning "no widening". Two
      queries replace three.
- [x] 1.3 Integration test (`//go:build integration`, `internal/platform/db`): seed a
      role→fuzzy chain (`A --duplicate_of_role--> B --duplicate_of_fuzzy--> C`, three distinct
      cities) and assert `C`'s closure geography carries all three. Cover a plain role cluster
      (unchanged behaviour), a closed member (excluded), and a marker cycle (terminates, returns
      nothing).
- [x] 1.4 Measure: `EXPLAIN ANALYZE` the whole-catalogue query against a prod-sized dataset and
      record the timing in `design.md` next to the risk it answers. If it is materially slower
      than `RoleClusterGeoAll`, stop and revisit before continuing. DONE 2026-09-01: the gate
      passes — 4.5x faster on a 235k-row slice, 2.5x on a 1M-row one. The whole catalogue
      cancelled at 20 min for BOTH queries, so the comparison had to be made on identical slices;
      that the query the rebuild runs TODAY is already >20 min is a finding in its own right.
      Planner cost estimates pointed the opposite way and were discarded — see design.md.

## 2. The index writers

- [x] 2.1 `cmd/reindex`: replace `buildClusterGeoLookup`'s `RoleClusterGeoAll` with the closure
      query and rekey the map by owner id; `splitJobs` looks up by `j.ID`. Drop the fingerprint
      plumbing that only fed geography (leave the reality-signal cluster counts alone — they still
      key by fingerprint).
- [x] 2.2 `cmd/search-drain/indexer.go`: same swap, keyed by job id. The `askGeo` gate was to become
      "the row owns at least one open duplicate", keeping its fail-open default. DONE differently:
      the gate is GONE. There is no cheap "represents nobody" test to feed it, and wanting one was
      the hazard — the gate had to default to asking anyway, because skipping the merge is
      destructive rather than conservative. Asking for every job in the wave deletes the reasoning
      instead of restating it; a row representing nobody answers with its own geography, so the
      merge is a no-op. Same removal in `linkimport` (2.3).
- [x] 2.3 `internal/ingest/linkimport`: same swap.
- [x] 2.4 Update `MergeClusterGeography`'s doc comment in `internal/search/search/document.go` to
      name the duplicate closure instead of the role cluster, and rename it if the new name reads
      better at all three call sites.
- [x] 2.5 Unit tests for all three writers: a document built for an owner carries the closure's
      union, and a writer that skips the union is caught (assert the merge happens on the
      incremental paths, not only the rebuild).
- [x] 2.6 Delete `RoleClusterGeo`, `RoleClusterGeoAll` and `RoleClusterGeoFor` from `jobs.sql`,
      `make sqlc`, and confirm nothing else references them. Delete
      `internal/platform/db/role_cluster_geo_integration_test.go` in the same step — its
      `clusterRow` fixture is what `closureRow` currently duplicates, and leaving the file behind
      turns a transitional copy into a permanent one.

## 3. The copies endpoint

- [x] 3.1 Replace `ListRoleClusterCopies` with a closure-based query: resolve the addressed slug to
      its ultimate owner, then list that owner's closure's open, non-private members ordered by
      location, with `COUNT(*) OVER()` pre-LIMIT as today.
- [x] 3.2 Integration test: copies of a fuzzy canon include the fuzzy-suppressed posting; copies
      requested from a SUPPRESSED posting return its owner's whole closure including the owner; a
      closed member and unrelated roles stay excluded; an out-of-range offset is an empty page.
- [x] 3.3 Confirm `internal/api/handler/copies.go`'s response shape is unchanged (`public_slug`,
      `location`, `apply_url`, `posted_at`, `meta.total`) so `web/` needs no diff, and that
      `JobRelated.svelte`'s `copiesTotal > 1` gate still reads correctly. CONFIRMED: shape
      untouched, no `web/` diff, and `openapi.yaml` does not describe this endpoint so there is
      nothing to update there. One edge shifts: a row representing nobody now answers with itself
      (`total: 1`) where an empty-fingerprint anchor used to answer with nothing (`total: 0`). The
      tab gate is `> 1`, so it stays hidden either way; only the standalone
      `/jobs/:slug/copies` page reads differently, and "1 openings" listing the job itself beats
      "0 openings" listing nothing. Pagination still goes through `pageParamsBounded`, which
      `TestOffsetIsParsedOnlyByTheSharedHelper` pins.

## 4. Releasing stale fuzzy markers

- [x] 4.1 Change `CompaniesWithFuzzyDedupCandidates` and `FuzzyDedupCandidateTitlesForCompany` from
      `duplicate_of IS NULL` to "not claimed by an exact pass"
      (`duplicate_of_aggregator IS NULL AND duplicate_of_role IS NULL`), so already-fuzzy-marked
      rows are re-decided.
- [x] 4.2 Teach `MarkFuzzyDuplicatesForCompany` to clear: take the full candidate id set alongside
      the assignment and write NULL for a candidate absent from the assignment, mirroring
      `RecomputeRoleDuplicatesForCompanies`'s `CASE`. Keep the `IS DISTINCT FROM` guard and the
      existing `search_outbox` / `search_delete_outbox` transition bookkeeping.
- [x] 4.3 Pass the candidate set through `collapseFuzzyDuplicatesForCompany` in
      `cmd/reindex/fuzzy.go`.
- [x] 4.4 Invert `TestFuzzyDedup_CandidateTitlesSkipAlreadyMarkedRows` in
      `internal/platform/db/fuzzy_dedup_integration_test.go` — an already-marked row IS a
      candidate now — and rename it to say so. CORRECTED: nothing to invert. That test marks
      `duplicate_of_role`, so it always described an EXACT-pass claim, which the new predicate
      still excludes; it was green before and after. The task assumed a blocker that was not
      there. Renamed to `..._SkipRowsClaimedByAnExactPass` to say what it actually pins, and a
      NEW test (`..._OfferRowsThisPassMarked`) covers the case that really changed.
- [x] 4.5 Integration tests for release: a marker clears when its canon closes and the row
      re-enters `search_outbox`; a marker clears when the descriptions diverge below the
      threshold; a second run with no changes writes nothing (idempotence).
- [x] 4.6 Rewrite the two false comments in `jobs.sql` (near lines 1630 and 1735) that claim the
      standard recompute reverses the fuzzy pass. CORRECTED: only ONE was false — the marker
      query's, now rewritten. The backfill comment checks out: the role recompute's `CASE` can
      write NULL, which frees the row, and the fuzzy pass re-decides it later in the same
      `refreshDuplicateMarkers` call. Verified rather than assumed, and left alone.

## 5. Verification and rollout

- [x] 5.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`,
      then the full `go test -tags=integration ./...` since behaviour changed. DONE 2026-09-01:
      gofmt silent, both vets clean, 175 packages green untagged, 183 green tagged, exit 0.
- [x] 5.2 `pnpm check:sql` is a no-op here (no migration added) — confirm, and confirm
      `golangci-lint run` reports nothing new. CONFIRMED: the diff touches no file under
      `migrations/`, so squawk has nothing to lint. `golangci-lint run --build-tags=integration
      --new-from-rev=origin/main` reports 3 findings, all in `me_credits_*` files this branch
      never touched — they read as "new" only because origin/main has moved ahead of the branch
      point. Nothing new in the 19 files this change owns.
- [x] 5.3 Write the rollout runbook into the change: stop `freehire-reindexw.timer`,
      `REINDEX_DEDUP_ONLY=1`, full `make reindex`, restart the timer. DONE — see design.md's
      Migration Plan, now carrying the measured expectations: the marker release will move
      public catalogue figures (42 633 rows were stranded behind closed owners), the first
      release run is the slow one, and a rollback does NOT re-hide the freed rows because the
      old code cannot reconsider a fuzzy marker at all.
- [ ] 5.4 After deploy, re-run the four URLs from issue #2225 and record the results: both affected
      searches return `total: 1`, both controls stay `total: 1`. Comment on the issue with the
      outcome.
