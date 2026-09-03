## 1. Schema

- [x] 1.1 Migration: create `boards` table (`provider, board, region, company, hub,
      tenants jsonb, url, status, submitted_by, surface, rejected_reason, created_at,
      activated_at`) with the CHECK constraints and the filtered unique index on
      `(provider, lower(board), region) WHERE status IN ('pending', 'active')`.
- [x] 1.2 Migration: create `board_submissions` table (`url, submitted_by, surface,
      created_at`) with the unique index on `url`.
- [x] 1.3 Regenerate sqlc (`make sqlc`) after adding the queries used below.

## 2. Insert-time validation

- [x] 2.1 Add `boardcatalog.Validate` (new package `internal/ingest/boardcatalog`, not
      `internal/ingest/sources` — it needs the boards-table concept, which `sources`
      doesn't have) taking a candidate plus the adapter registry, reusing
      `sources.Config.Validate` for the unknown-provider/empty-board checks. Duplicate
      detection is NOT a pure check — it's the `boards_identity_key` unique index,
      enforced by `boardcatalog.Insert`/`Repository.InsertRow` (see 2.2).
- [x] 2.2 Tests: unit tests for `Validate` (unknown provider rejected, empty board
      rejected for a board-based provider, empty board accepted for `boardless`,
      valid entry accepted); integration tests for `Repository`
      (`repository_integration_test.go`) covering duplicate-of-live-row rejected
      (`ErrDuplicateBoard`) and resubmission accepted after `rejected` or `retired` —
      the latter is why `boards_identity_key` filters on `status IN ('pending',
      'active')` rather than `status <> 'retired'` (see the spec fix committed
      alongside this change).

## 3. Backfill

- [x] 3.1 `cmd/backfill-board-catalog`: parse every `sources/*.yml` entry (reusing the
      existing YAML parsing one last time) and insert each into `boards` at
      `status='active', activated_at=now()`, idempotent via the unique index.
- [x] 3.2 Test: running the backfill twice inserts no duplicate rows.
- [ ] 3.3 MANUAL/OPERATOR STEP — not run from this workspace: run it against prod once
      (`DATABASE_URL` only); confirm row count matches the current YAML entry count.
      Do this only once §4-§7 are also deployed (design.md's Migration Plan step 2-3 are
      adjacent deploys), and before §9's YAML deletion.

## 4. `cmd/ingest` reads from `boards`

- [x] 4.1 Add a DB-backed loader: `boardcatalog.LoadForProvider` (in `boardcatalog`, not
      `internal/ingest/sources` — `sources` can't import `boardcatalog`, which imports
      `sources` for `Config.Validate`; putting it there would be an import cycle),
      calling `Repository.ListActiveForProvider` (`status IN ('pending','active')`) and
      mapping each `Board` to a `sources.CompanyEntry` via the new `Board.CompanyEntry()`.
- [x] 4.2 Changed `cmd/ingest`'s argument from a file path / `SOURCES_FILE` to a provider
      name (`INGEST_PROVIDER`); wired to `boardcatalog.LoadForProvider`. `--shard=i/n`
      unchanged — it still calls `Config.Shard`, now over DB-sourced entries.
- [x] 4.3 `cmd/ingest <provider>` crawling exactly that provider's `pending`/`active` rows
      is proven by `boardcatalog`'s `TestListActiveForProviderExcludesRejectedAndRetired`
      (the query) plus `TestLoadForProviderMapsBoardsToCompanyEntries` (the mapping) — the
      CLI arg-parsing glue itself (extracting the provider from argv/env) is a few lines
      mirroring the pre-existing, likewise-untested shard-parsing pattern; not
      independently tested, at parity with what was there before.
- [x] 4.4 No new test needed: `Config.Shard` (`internal/ingest/sources/shard_test.go`,
      pre-existing) already proves the round-robin/no-split partitioning over
      `[]CompanyEntry`, and it is unchanged — only where those entries come from changed,
      proven faithful by 4.1's loader test.

## 5. `pending` → `active` transition

- [ ] 5.1 In `pipeline.Runner`, alongside the existing `board_health` outcome write, flip
      a `pending` board to `active` (stamping `activated_at`) on a crawl that completes
      without a board-level error.
- [ ] 5.2 Integration test: a `pending` board's first successful crawl becomes `active`;
      a failing crawl leaves it `pending`.

## 6. `cmd/add-board`

- [ ] 6.1 `cmd/add-board`: report-by-default, `--apply` to write. Add mode inserts at
      `status='active'` through the same validation function as §2. Retire mode
      (`--retire provider/board[/region]`) sets an existing row's status to `retired`
      without deleting it.
- [ ] 6.2 Tests: dry run writes nothing; `--apply` inserts/retires; re-running add is a
      no-op via the unique index; an invalid add reports the same validation reason as a
      crowdsourced submission would.

## 7. Contribution flow moves to `boards`/`board_submissions`

- [ ] 7.1 Rewrite `internal/ingest/contribution`'s `Record` to insert into `boards` at
      `status='pending'` (via the §2 validation function) instead of `link_contributions`,
      preserving the existing reward/idempotency behavior.
- [ ] 7.2 Rewrite `RecordReview` to insert into `board_submissions` instead of
      `link_contributions` with `source IS NULL`.
- [ ] 7.3 Rewrite `ListByUser` to read a caller's rows from `boards` (mapping
      `pending`/`active`/`rejected`) unioned with their `board_submissions` rows (status
      `review`), newest first.
- [ ] 7.4 Update existing `internal/ingest/contribution` tests for the new backing
      tables; add a triage helper (or document the manual `psql` step) that deletes a
      `board_submissions` row and inserts the resolved `(provider, board)` into `boards`.
- [ ] 7.5 Confirm the "board already in the catalogue" and "board already contributed"
      checks (existing requirements, unmodified) now read `boards` instead of
      `link_contributions`/`jobs`.

## 8. Cutover

- [ ] 8.1 Update deployment cron/systemd units from one-timer-per-file to
      one-timer-per-provider-name (same count).
- [ ] 8.2 Deploy §4-§7 together (per design.md's Migration Plan step 3 — `cmd/ingest`
      cannot read two sources at once).
- [ ] 8.3 Confirm a full crawl cycle of every provider completes clean against `boards`
      in prod before proceeding.

## 9. Retire the old path

- [ ] 9.1 Delete `sources/*.yml` and `cmd/validate-sources`.
- [ ] 9.2 Remove the "Validate sources" CI step.
- [ ] 9.3 Remove the `onboard-contributions` skill.
- [ ] 9.4 Remove `internal/ingest/sources/config.go`'s YAML parsing
      (`LoadConfig`/`ParseConfig`/`ParseRawEntries`/`dedupeBoards`) and delete
      `cmd/backfill-board-catalog` (one-off, no longer needed once it has run in prod).
- [ ] 9.5 Migration: drop `link_contributions`.

## 10. Review

- [ ] 10.1 Run `go vet -tags=integration ./...` and the full integration suite once
      behavior in §4-§7 is complete.
- [ ] 10.2 `gofmt -l .` clean; `go vet ./...`; `go test ./...`.
