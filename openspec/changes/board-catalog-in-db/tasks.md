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

- [x] 5.1 Wired into `cmd/ingest`'s `boardHealth.RecordSuccess` (not `pipeline.Runner`
      itself — `RecordSuccess` is the concrete adapter `pipeline.Runner` already calls on
      a board-level success, so piggybacking there needed no change to `pipeline.go` or
      the `pipeline.BoardHealth` port). Calls `boardcatalog.Repository.Activate` right
      after the existing `board_health` write.
- [x] 5.2 Integration tests (`board_health_activation_integration_test.go`): a `pending`
      board's first successful crawl becomes `active` with `ActivatedAt` set; a success
      for a board with no `boards` row is harmless (no error); an already-`active` board
      is not re-activated (its `ActivatedAt` is unchanged). A failing crawl leaving a
      board `pending` is implied — `RecordFailure` never calls `Activate` — and not
      separately tested, since `RecordFailure` and `RecordSuccess` are already mutually
      exclusive per crawl outcome in `pipeline.Runner`.

## 6. `cmd/add-board`

- [x] 6.1 `cmd/add-board`: report-by-default, `--apply` to write. Add mode
      (`--provider --board --company [--region] [--hub] [--tenants=k:v,...]`) inserts at
      `status='active'` through `boardcatalog.Insert`/`Validate` (same as §2). Retire mode
      (`--retire --provider --board [--region]`) sets an existing live row's status to
      `retired` without deleting it. Also added `--rename` mode
      (`--rename --provider --board --company [--region]`) — needed because of a design
      gap found while starting §7: a crowdsourced board (no network fetch, see
      `link-contributions`) has no real company name, yet board-based adapters
      (greenhouse/lever/ashby) write the catalog's company verbatim as every crawled
      job's employer. Resolved (user decision) as: seed with
      `boardcatalog.PlaceholderCompany(board)` (a humanized slug, e.g. "acme-corp" ->
      "Acme Corp") and let a curator fix it via `--rename` once they see the pending
      board. New `Repository.Rename` + `UpdateBoardCompany` query; recorded as its own
      requirement in `specs/board-catalog/spec.md`.
- [x] 6.2 Tests: `addBoard`/`retireBoard` (the DB-touching cores, split out from the
      `worker.Bootstrap`-calling `runAdd`/`runRetire` CLI wrappers — a test needs to hand
      them a throwaway `testdb.Pool`, not the environment's real `DATABASE_URL`, which
      `worker.Bootstrap` would otherwise read; found by a first version of these tests
      failing with "relation boards does not exist" against a real dev database) cover:
      `--apply` inserts/retires, retire doesn't delete the row, re-running add is a no-op
      via `ErrDuplicateBoard`, retiring a nonexistent board reports not-found. The dry-run
      and invalid-candidate paths are covered on the real `runAdd`/`runRetire` (they never
      reach the database either way, so the environment's real one is harmless to touch).

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
