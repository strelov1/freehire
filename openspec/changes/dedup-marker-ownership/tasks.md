## 1. Answer the open questions before writing schema

- [ ] 1.1 Count on prod, off-peak and with a `statement_timeout`, how many open rows would
      carry a marker in more than one owned column under the seeding rule of Decision 4.
      Expected zero; record the actual number in `design.md` and revisit Decision 2 if it is
      not.
- [ ] 1.2 Grep the module for every write to `duplicate_of` — queries, Go call sites, and any
      `migrations/*.sql` that sets it — and confirm the writer list is exactly the three
      passes plus `cmd/ingest/store.go` and `internal/linkimport/linkimport.go`.
- [ ] 1.3 Record the current prod baseline for re-marked rows per pass per run from the
      `freehire-reindex-dedup-only` journal, so the post-deploy acceptance check has a
      before-picture that outlives the journal's retention.

## 2. Schema

- [ ] 2.1 Migration: add `duplicate_of_role`, `duplicate_of_aggregator`, `duplicate_of_fuzzy`
      as nullable `bigint` on `jobs`, with the same FK shape `duplicate_of` carries today.
      Catalog-only, no rewrite.
- [ ] 2.2 Migration: the `BEFORE INSERT OR UPDATE` trigger deriving `duplicate_of` from the
      three, per Decision 1 — pure `NEW`-local PL/pgSQL, no query, early return when no owned
      column changed. Ships in a separate file from 2.1 so the backfill can run between them.
- [ ] 2.3 Integration test: the trigger derives the COALESCE in precedence order, ignores a
      direct write to `duplicate_of`, and is a no-op when no owned column changed.

## 3. Backfill

- [ ] 3.1 `cmd/backfill-duplicate-marker-owner`: chunked keyset pass over `id`, seeding by
      shape per Decision 4, `IS DISTINCT FROM`-guarded, resumable, `DATABASE_URL` only —
      modelled on `cmd/backfill-slug-folded`.
- [ ] 3.2 Integration test: an aggregator row pointing at a non-aggregator canon seeds the
      aggregator column, every other marked row seeds the role column, an unmarked row seeds
      nothing, and a second run writes zero rows.
- [ ] 3.3 The reconcile sweep of Decision 5 step 4 — rows with `duplicate_of` set and all
      three owned columns NULL — as a flag on the same worker.

## 4. Passes write their own column

- [ ] 4.1 `RecomputeRoleDuplicatesForCompanies` writes `duplicate_of_role`; its `target` CTE
      and `IS DISTINCT FROM` guard move to that column.
- [ ] 4.2 `SuppressAggregatorDuplicatesForCompanies` writes `duplicate_of_aggregator`.
- [ ] 4.3 `MarkFuzzyDuplicatesForCompany` writes `duplicate_of_fuzzy`. Its scoping predicate
      keeps reading the derived `duplicate_of` — it wants "still canonical", which is the
      derived question.
- [ ] 4.4 Rename `MarkJobDuplicateOf` to `MarkJobDuplicateOfRole` and point it at the role
      column; update `cmd/ingest/store.go` and `internal/linkimport/linkimport.go`.
- [ ] 4.5 `make sqlc`, then fix the two call sites and anything the regenerated `querier.go`
      breaks.

## 5. Prove the defect is gone

- [ ] 5.1 Integration test reproducing the ping-pong on the old schema's terms: mark a row
      via the aggregator pass, run the role recompute over the same company with that row a
      singleton in its role cluster, and assert the suppression survives. This is the test
      that fails before the change.
- [ ] 5.2 Integration test: a full `refreshDuplicateMarkers` cycle over an unchanged fixture
      catalogue re-marks zero rows on the second run, across all three passes.
- [ ] 5.3 Integration test: running the three passes in a different order produces the same
      end state.
- [ ] 5.4 A test walking `internal/db/queries/*.sql` that fails if any statement assigns
      `duplicate_of` directly, in the shape of the existing walk that keeps the legal-form
      vocabulary single-sourced.

## 6. Ship

- [ ] 6.1 `gofmt -w`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, then
      the tagged suite for `internal/db` and `cmd/reindex`.
- [ ] 6.2 Apply 2.1 on prod, run the backfill under `systemd-run`, apply 2.2, run the
      reconcile sweep, then deploy the code — the order of Decision 5, with the marker-refresh
      timer stopped for the window between the backfill finishing and the deploy landing.
- [ ] 6.3 After the first post-deploy refresh cycle absorbs the seeding correction, confirm
      the next cycle reports near-zero re-marked rows for all three passes against the 1.3
      baseline. This is the acceptance criterion.
- [ ] 6.4 Confirm `reindex --since` still behaves with a working set roughly a third smaller,
      per the last risk in `design.md`.
