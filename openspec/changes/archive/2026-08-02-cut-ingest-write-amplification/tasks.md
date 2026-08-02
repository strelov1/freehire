## 1. Guard the invariant the whole change rests on

- [x] 1.1 Add a test in `internal/jobhash` asserting every input `RoleFingerprint` reads is
      also an input `jobhash.Of` reads, so a fingerprint can never go stale behind a matching
      `content_hash`. Follow the shape of the existing `TestOfRow_CarriesEveryFieldTheHashReads`.
- [x] 1.2 The same guard for every other column the cheap path skips: a test walking
      `jobderive.Input` through the real composition (`job.New` → `Fields().UpsertParams()`) and
      failing when a mutation moves any written column without moving the cheap path's match
      key. Outcome: `Cities` was the only gap of the fourteen — `Source` and `ExternalID` needed
      no exemption, since both move `PublicSlug`, which is hashed. Resolved by putting `cities`
      in the match key rather than in `jobhash.Of`: hashing it would change every stored hash at
      once and make the first crawl after deploy rewrite and re-index the whole catalogue. The
      test now states the key and is the authority on it, so a later unhashed derived column
      fails there. See design.md, "The match key is `content_hash` AND `cities`".

## 2. The cheap write

- [x] 2.1 Add `RefreshUnchangedJob` to `internal/db/queries/jobs.sql`: `UPDATE jobs SET
      last_seen_at = now()` matched on `(source, external_id, content_hash, cities)` and guarded
      by `closed_at IS NULL`, returning the row. `cities` is in the key because task 1.2 proved
      it is the one written column the content hash does not cover; the query comment names
      TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites as its guard. Run `make sqlc`.
- [x] 2.2 Integration test (`internal/db`, `//go:build integration`): the query advances
      `last_seen_at`, leaves every other column including `updated_at` untouched, and returns
      no row for a closed posting, a mismatched hash, or a mismatched `cities`.

## 3. The ingest seam

- [x] 3.1 In `cmd/ingest/store.go`, extract the `qtx.UpsertJob` call in the private `save`
      behind a helper that tries `RefreshUnchangedJob` first (through `qtx`, INSIDE the existing
      transaction, so the refresh and the enrichment enqueue still commit together) and falls
      back on `pgx.ErrNoRows`. The cheap branch returns only four columns, so the helper needs an
      explicit return type rather than a `db.UpsertJobRow`: a synthesised partial `db.Job` would
      hand a later reader `""` for `Title` instead of failing. `Save` and `SaveWithApplyForm`
      both inherit it.
- [x] 3.2 Integration test: re-ingesting an identical posting reports neither inserted nor
      changed, issues no index push and no role-cluster lookup, and still records the company
      into the crawled-set and runs the enrichment enqueue.
- [x] 3.3 Integration test: re-ingesting a **closed** posting with identical content reopens
      it — `closed_at` and the close record cleared, strikes reset, `updated_at` stamped. This
      is the regression guard for the `closed_at IS NULL` predicate.
- [x] 3.4 Integration test: `SaveWithApplyForm` on an unchanged posting still writes the
      apply form.

## 4. `updated_at` means "content changed"

- [x] 4.1 Change `TouchJob` (`jobs.sql`) to stamp `updated_at` only when it reopens:
      `updated_at = CASE WHEN closed_at IS NOT NULL THEN now() ELSE updated_at END`.
      Run `make sqlc`.
- [x] 4.2 Integration test: `TouchJob` on an open row advances `last_seen_at` and leaves
      `updated_at`; on a closed row it reopens and stamps `updated_at`.

## 5. The company row

- [x] 5.1 Guard the `company_upsert` CTE inside `UpsertJob` only with
      `WHERE companies.name IS DISTINCT FROM EXCLUDED.name`. Leave the CTEs in
      `UpsertManualJob` and `UpdateManualJob` alone. Run `make sqlc`.
- [x] 5.2 Integration test: ingesting a posting for an existing company under an unchanged
      name leaves `companies.updated_at` untouched; a renamed company still updates both
      `name` and `updated_at`.

## 6. Make the reach observable

- [x] 6.1 Count cheap-path versus full-path writes per ingest run and log the pair once at
      run end, attributed to the provider. DEVIATION, deliberate: this does NOT go through
      `pipeline.RunStats`. The Runner only ever sees `error` back from `Store.Save`, so it cannot
      learn which write ran without changing the Store interface. `writeTally` instead mirrors
      `crawledSet` — the collector already established on this exact path for the same reason —
      and emits its own line beside the run summary. The hydrating `Touch` path is counted too:
      it IS the cheap write for those providers, and omitting it would report a false 0%.
- [x] 6.2 Test: a run that re-saw only unchanged postings reports a full cheap-path share; a
      run whose postings all changed reports zero, rather than reporting nothing.

## 7. Storage parameters

- [x] 7.1 New migration setting `fillfactor = 90` and `autovacuum_vacuum_scale_factor = 0.02`
      on `jobs`, in its own file so it deploys separately. Confirm the next free migration
      number against prod, not just against the repo.

## 8. Verify

- [x] 8.1 `go build ./... && go vet ./... && go test ./...`, then
      `go vet -tags=integration ./...` and `go test -tags=integration ./internal/db/ ./cmd/ingest/`.
      All pass. NOTE, pre-existing and NOT fixed here: running several container-using packages
      in one `go test` invocation is flaky on a loaded machine — roughly one run in three dies
      with `clone hire_template: … connection reset by peer` in unrelated tests (`ghost_*`), and
      `-p 1` is stable. The harness closes each pool via `t.Cleanup`, so tests do not accumulate
      connections; this is concurrent testcontainer startup, not test count. Per-package runs —
      the form CLAUDE.md documents — are reliable.
- [x] 8.2 Snapshot `pg_stat_user_tables` for `jobs` and `companies` on prod and record the
      numbers in the change before releasing, so the post-release comparison has a baseline.
      Recorded in `baseline.md` (2026-08-02T21:11:43Z).
- [x] 8.3 Confirm on prod (`\d jobs`) that no index covers `last_seen_at`. The whole HOT premise
      rests on it, and git is not authoritative: `migrations/0043_jobs_user_fk_idx.sql` records
      that its two indexes were applied by hand with `CREATE INDEX CONCURRENTLY`. Confirmed:
      21 indexes, matching git exactly, none referencing `last_seen_at` as a key column or in a
      partial predicate. See `baseline.md`.
