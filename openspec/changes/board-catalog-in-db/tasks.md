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

- [x] 7.1 Rewrote `internal/ingest/contribution`'s `Record` to insert into `boards` at
      `status='pending'` via `boardcatalog.Insert` (validation + `PlaceholderCompany`
      seeding), instead of `link_contributions`. Reward/idempotency behavior unchanged —
      `boardcatalog.ErrDuplicateBoard` maps to the same `ErrBoardAlreadyContributed` the
      caller already handles.
- [x] 7.2 Rewrote `RecordReview` to insert into `board_submissions` (`InsertBoardSubmission`)
      instead of `link_contributions` with `source IS NULL`; its unique `url` index maps a
      duplicate the same way.
- [x] 7.3 Rewrote `ListByUser` to merge `boardcatalog.Repository.ListBySubmitter` (mapped
      to `pending`/`active`/`rejected`) with `board_submissions` rows for that user
      (mapped to `review`), sorted newest-first in Go (two tables, no single query).
- [x] 7.4 `Service`/`contribution_test.go` (the fake-`Repository` unit tests) needed no
      changes — they were already decoupled from the backing store. Rewrote
      `repository_integration_test.go` in full against real Postgres: duplicate-of-a-live-board
      rejected, resubmission accepted after `rejected`/`active` (renamed from
      `onboarded`), the concurrent-duplicate race, `RecordReview` + its own dedup, and
      `ListByUser` merging both tables while staying scoped to the caller. No triage
      helper added — today's flow is a manual `psql` step either way (delete the
      `board_submissions` row, `INSERT` into `boards`), same shape as before.
- [x] 7.5 `BoardTracked`/`CompanyForBoard`/`BoardByGreenhouseJobID`/`BoardByAshbyJobID` are
      unchanged — they always read `jobs`/`companies`, never `link_contributions`, so
      "already in the catalogue" was never affected by this migration. "Already
      contributed" now reads through `boardcatalog`'s `boards_identity_key` (§7.1) instead
      of `link_contributions`' unique index — same semantics, different table.
- [x] Also fixed along the way: `boards`/`board_submissions`' `surface` CHECK was missing
      `discord`/`unknown` (contribution's actual surface vocabulary) — caught by these
      tests failing on `boards_surface_check`, not by inspection. Both unreleased
      migrations edited in place (not re-migrated anywhere yet). Also updated the
      module-layering block table (`boardcatalog` → `ingest`) and `.gitignore`
      (`/add-board`, `/backfill-board-catalog`) — two repo-wide guard tests caught these
      on a full `go test ./...` run. Also updated `web/src/lib/types.ts`'s
      `ContributionStatus` and one `api-spec.ts` doc example from `'onboarded'` to
      `'active'` (frontend type-check not run — no `node_modules` in this worktree; the
      change is a single literal in a union type with no other references).

## 8. Cutover — DONE, deployed to prod 2026-09-03

Executed live once the code (§1-§7) was merged (freehire#2357) and CI was green. Sequenced
by hand with `AUTODEPLOY` (host2's freehire-ops#64 CI-triggered auto-release, discovered
armed mid-cutover) temporarily disarmed, so the code deploy and the ops-side timer changes
landed in the same controlled window instead of racing a 10-minute poll:

- [x] 8.1 `freehire-ops` updated: `freehire-ingest@.service` and all 7 shard templates
      (workday/eightfold/oracle/paylocity/join, plus two that had drifted onto host2
      outside git — `dayforce`/`workstream`, found and reconciled during this cutover)
      now pass the provider name instead of a board-file path. `custom.yml`'s ~24
      providers split from one bundled 3h timer into 24 individual hourly timers (the 3h
      cadence existed because bundling 24 API calls took a while, not because any one is
      slow). `cmd/add-board` and `cmd/backfill-board-catalog` added to `release.sh`'s
      worker-binary build list.
- [x] 8.2 Deployed via `release.sh freehire` on host2 (green): `cmd/migrate` applied
      `0123_boards.sql`/`0124_board_submissions.sql`, then `cmd/backfill-board-catalog`
      ran manually — **157,552 rows inserted, 0 failed** across 237 providers — before the
      new systemd timers went live.
- [x] 8.3 Confirmed within the hour: 26,711 of 143,849 `board_health` rows already
      recrawled successfully against `boards` (fresh `last_run_at`/`last_success_at`,
      `consecutive_failures=0`) and 11,989 jobs written since the deploy. Full coverage of
      every provider takes up to 24h (paylocity's cadence) — the daily and 3h-cadence
      providers are the tail still to confirm, not a sign of a problem.

## 9. Retire the old path — still deliberately not done

Not executed yet, even though §8 is live: §8.3's confirmation is partial (see above) — the
slower-cadence providers (daily `apple`/`taleo`, every-6h `reed`, paylocity's 24h cycle)
have not yet completed a full pass against the new code. Deleting `sources/*.yml` before
every provider has proven itself would remove the fallback (git history) with no upside —
`cmd/ingest` already reads only from `boards`, so nothing currently depends on the files.
Revisit once a full 24h+ cycle confirms no provider regressed. Once confirmed:

- [ ] 9.1 Delete `sources/*.yml` and `cmd/validate-sources`.
- [ ] 9.2 Remove the "Validate sources" CI step.
- [ ] 9.3 Remove the `onboard-contributions` skill.
- [ ] 9.4 Remove `internal/ingest/sources/config.go`'s YAML parsing
      (`LoadConfig`/`ParseConfig`/`ParseRawEntries`/`dedupeBoards`) and delete
      `cmd/backfill-board-catalog` (one-off, no longer needed once it has run in prod).
- [ ] 9.5a Run `cmd/backfill-link-contributions --apply` in prod. **9.5 destroys data until
      this has run.** 9.1-9.4 moved the read and write paths, not the rows: measured
      2026-09-04, `link_contributions` held 404 contributions from 11 users, of which
      `board_submissions` held none of the 26 unclassified URLs and `boards` held neither
      of the 2 pending boards. The worker also reports the `onboarded` contributions whose
      board is absent from the catalog (9 on prod) — those are boards nothing crawls, and
      they need resolving, not dropping.
- [ ] 9.5 Migration: drop `link_contributions` — only after 9.5a reports a clean run.

## 10. Review

- [x] 10.1 `go vet -tags=integration ./...` and the full `go test -tags=integration ./...`
      both clean. This caught three `internal/api/handler` integration test files
      (`discord_integration_test.go`, `telegram_contribution_integration_test.go`,
      `resolve_job_integration_test.go`) with raw SQL against `link_contributions` —
      updated to `boards`/`board_submissions` (and `source` → `provider`); the
      review-status assertion (no `status` column on `board_submissions` — a row's mere
      existence there IS the review state) was rewritten as an existence check rather than
      a status-string read.
- [x] 10.2 `gofmt -l` clean on every changed file; `go vet ./...` and `go test ./...`
      clean. A full (non-`-tags=integration`) `go test ./...` also caught two repo-wide
      guard tests unrelated to the integration suite: the module-layering block table
      (`boardcatalog` had no block assignment) and `.gitignore` (`/add-board`,
      `/backfill-board-catalog` un-anchored, i.e. a local build of either could be
      committed by accident) — both fixed.
