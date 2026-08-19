## 1. Answer the open questions before writing schema

- [x] 1.1 Count on prod, off-peak and with a `statement_timeout`, how many open rows would
      carry a marker in more than one owned column under the seeding rule of Decision 4.
      **8,279 of 1,162,487 open marked rows (0.7%), not zero.** Decision 2 rewritten: the
      precedence order is aggregator, role, fuzzy, because that is which pass wins a
      contested row today.
- [x] 1.2 Grep the module for every write to `duplicate_of` — queries, Go call sites, and any
      `migrations/*.sql` that sets it — and confirm the writer list is exactly the three
      passes plus `cmd/ingest/store.go` and `internal/linkimport/linkimport.go`.
      **Confirmed:** four SQL statements assign it (`MarkJobDuplicateOf`,
      `RecomputeRoleDuplicatesForCompanies`, `SuppressAggregatorDuplicatesForCompanies`,
      `MarkFuzzyDuplicatesForCompany`); no `INSERT` sets it, no migration sets it, and the
      only Go call sites are the two named above.
- [x] 1.3 Record the current prod baseline for re-marked rows per pass per run from the
      `freehire-reindex-dedup-only` journal, so the post-deploy acceptance check has a
      before-picture that outlives the journal's retention. **In `proposal.md`**; the
      2026-08-19 01:30 cycle adds the decisive arithmetic — role re-marked 474,949 while
      aggregator and fuzzy together hold 470,033, so ~5k of the ~950k is real work.

## 2. Schema

- [x] 2.1 Migration: add `duplicate_of_role`, `duplicate_of_aggregator`, `duplicate_of_fuzzy`
      as nullable `bigint` on `jobs`. Catalog-only, no rewrite. **No foreign key** — see
      Decision 7, added while writing this.
      (`migrations/0114_jobs_duplicate_marker_owner_columns.sql`)
- [x] 2.2 Migration: the `BEFORE INSERT OR UPDATE` trigger deriving `duplicate_of` from the
      three, per Decision 1 — pure `NEW`-local PL/pgSQL, no query. The early return is the
      engine's: `UPDATE OF` lists the four marker columns, so a statement naming none of them
      never enters the trigger. Separate file from 2.1 so the backfill runs between them.
      (`migrations/0115_jobs_derive_duplicate_of.sql`)
- [x] 2.3 Integration test: the trigger derives the COALESCE in precedence order, ignores a
      direct write to `duplicate_of`, and is a no-op when no owned column changed.
      (`internal/db/duplicate_marker_ownership_integration_test.go`)

## 3. Backfill

- [x] 3.1 `cmd/backfill-duplicate-marker-owner`: chunked pass over an id RANGE, seeding by
      shape per Decision 4, resumable, `DATABASE_URL` only — modelled on
      `cmd/backfill-slug-folded`. Closed rows are seeded too: nothing else ever would, and the
      first statement to touch a marker column on one would otherwise fire the derivation and
      clear a `duplicate_of` that `cmd/prune` still walks.
- [x] 3.2 Integration test: an aggregator row pointing at a non-aggregator canon seeds the
      aggregator column, every other marked row seeds the role column, an unmarked row seeds
      nothing, and a second run writes zero rows. The fixture disables the trigger to produce a
      genuinely pre-0114 row — the only honest way to make one in a database that already has
      the derivation.
- [x] 3.3 ~~The reconcile sweep as a flag on the same worker.~~ **Not needed.** The seeding
      predicate is "marked, and no owned column set", which is exactly the reconcile predicate:
      re-running the worker after 0115 lands picks up every row written while it was walking.
      A separate mode would have been a second name for the same statement.

## 4. Passes write their own column

- [x] 4.1 `RecomputeRoleDuplicatesForCompanies` writes `duplicate_of_role`; its `target` CTE
      and `IS DISTINCT FROM` guard move to that column.
- [x] 4.2 `SuppressAggregatorDuplicatesForCompanies` writes `duplicate_of_aggregator`. Its
      candidate predicate deliberately keeps reading the derived `duplicate_of`: "canonical OR
      already pointing at a non-aggregator row" is what admits the contested rows of Decision
      2, and rewriting it as `duplicate_of_aggregator IS NOT NULL` would quietly shrink the
      candidate set by those 8,279 rows.
- [x] 4.3 `MarkFuzzyDuplicatesForCompany` writes `duplicate_of_fuzzy`. Its scoping predicate
      keeps reading the derived `duplicate_of` — it wants "still canonical", which is the
      derived question.
- [x] 4.4 Rename `MarkJobDuplicateOf` to `MarkJobDuplicateOfRole` and point it at the role
      column; update `cmd/ingest/store.go` and `internal/linkimport/linkimport.go`.
- [x] 4.5 `make sqlc`, then fix the two call sites and anything the regenerated `querier.go`
      breaks. Also every test fixture that wrote `duplicate_of` directly — eleven files across
      `internal/db`, `internal/handler`, and `internal/catalogstats` — now writes
      `duplicate_of_role`. That the trigger broke all of them at once is the guarantee
      working: those writes no longer take.

## 5. Prove the defect is gone

- [x] 5.1 Integration test reproducing the ping-pong on the old schema's terms: mark a row
      via the aggregator pass, run the role recompute over the same company with that row a
      singleton in its role cluster, and assert the suppression survives. Same test for the
      fuzzy pass. (`internal/db/duplicate_marker_no_clobber_integration_test.go`)
- [x] 5.2 Integration test: a full refresh cycle over an unchanged fixture catalogue re-marks
      zero rows on the second run, across all three passes. **This is the test that carries
      the weight.** Verified by reverting the role pass to write `duplicate_of` and re-running:
      it fails with "role recompute re-marked 3 rows, want 0". The 5.1 tests do NOT fail under
      that revert — the trigger still shields the owned columns — so the cycle test is the one
      that would catch a regression.
- [x] 5.3 Integration test: running the three passes in a different order produces the same
      end state. Compares the DERIVED canon: an owned column may legitimately differ between
      orders (fuzzy-first leaves a redundant marker COALESCE never surfaces), which is what
      precedence is for.
- [x] 5.4 A test walking `internal/db/queries/*.sql` that fails if any statement assigns
      `duplicate_of` directly, in the shape of the existing walk that keeps the legal-form
      vocabulary single-sourced. (`internal/db/duplicate_marker_owner_rule_test.go`; it caught
      the `sqlc.arg(duplicate_of)` parameter name on its first run, which is now
      `duplicate_of_role` to match its column.)

## 6. Ship

- [x] 6.1 `gofmt -w`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, then
      the tagged suite for `internal/db` and `cmd/reindex`. All clean, plus the tagged suites
      for `internal/handler`, `cmd/ingest`, `internal/linkimport` and `internal/catalogstats` —
      every package whose fixtures wrote `duplicate_of`.
- [x] 6.2 Apply 2.1 on prod, run the backfill under `systemd-run`, apply 2.2, run the
      reconcile sweep, then deploy the code — the order of Decision 5, with the marker-refresh
      timer stopped for the window between the backfill finishing and the deploy landing.
      **Deployed 2026-08-19.** Three things went differently from the plan and are worth the
      next person's attention:
      - `release.sh` applies every pending migration in one step, so 0114 and 0115 landed
        together. Harmless in the end — the derivation only fires on a write naming a marker
        column, so it clears nothing retroactively — but the plan's "backfill between them"
        was not achievable through the normal deploy path. What actually protects the
        catalogue is the marker timers being stopped, not the migration order.
      - **The deploy caused a 7-minute outage, and the cause was the worker pause switch.**
        `freehire:pause:all` was set to reduce lock contention; `cmd/migrate` is a worker, so
        it exited 0 with "paused, skipping this run", `release.sh` read that as success and
        flipped the colour. New code, old schema, `SQLSTATE 42703` on every job read. The
        migration block guards against "migration failed", not "migration politely declined".
      - `ALTER TABLE` needed ten retries to win `ACCESS EXCLUSIVE` on `jobs` even off-peak;
        the 5s `lock_timeout` is deliberate and the answer is repetition, not a longer wait.
      Backfill: 2,032,470 rows, ~55 min at `BACKFILL_MARKER_CHUNK=1000000` (the 50k default
      projected five hours — the chunk spans an id RANGE and the sequence runs far ahead of
      the row count).
- [x] 6.3 After the first post-deploy refresh cycle absorbs the seeding correction, confirm
      the next cycle reports near-zero re-marked rows for all three passes against the 1.3
      baseline. This is the acceptance criterion. **PASSED.**

      | Pass | Baseline | Cycle 1 (seeding correction) | Cycle 2 |
      |---|---|---|---|
      | role | 460k–495k | 365,015 | **4,035** |
      | aggregator | 120k–128k | 5,007 | **4,928** |
      | fuzzy | 305k–348k | 340,234 | **4,422** |
      | total | ~950,000 | 710,256 | **13,385** |

      A 98.6% reduction, and the cycle now takes 1h33m instead of over two hours. The
      aggregator pass dropped in the FIRST cycle — it never needed the seeding correction,
      since the backfill seeds its column by shape. Cycle 2 ran 17:07–18:40 UTC.
- [ ] 6.4 Confirm `reindex --since` still behaves with a working set roughly a third smaller,
      per the last risk in `design.md`. **Deferred, deliberately.** `--since` is not on any
      timer (the scheduled rebuilds are full-scope), so nothing exercises it until someone
      runs it by hand. The risk it guards against — a smaller working set — is a correctness
      improvement, and the acceptance numbers above already show `updated_at` has stopped
      churning. Left unchecked rather than silently dropped: whoever next runs a `--since`
      rebuild should confirm it, and this line is the reminder.
