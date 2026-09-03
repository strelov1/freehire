## 1. Schema

- [x] 1.1 Migration: create `boards` table (`provider, board, region, company, hub,
      tenants jsonb, url, status, submitted_by, surface, rejected_reason, created_at,
      activated_at`) with the CHECK constraints and the filtered unique index on
      `(provider, lower(board), region) WHERE status IN ('pending', 'active')`.
- [x] 1.2 Migration: create `board_submissions` table (`url, submitted_by, surface,
      created_at`) with the unique index on `url`.
- [x] 1.3 Regenerate sqlc (`make sqlc`) after adding the queries used below.

## 2. Insert-time validation

- [ ] 2.1 Add a validation function in `internal/ingest/sources` taking a candidate
      `(provider, board, region)` plus the adapter registry, returning nil or a typed
      reason: unknown provider, empty board on a non-`boardless` provider, or duplicate
      of an existing non-`retired` row.
- [ ] 2.2 Unit tests: unknown provider rejected, empty board rejected for a board-based
      provider, empty board accepted for a `boardless` provider, duplicate of a live row
      rejected, duplicate of a `retired` row accepted.

## 3. Backfill

- [ ] 3.1 `cmd/backfill-board-catalog`: parse every `sources/*.yml` entry (reusing the
      existing YAML parsing one last time) and insert each into `boards` at
      `status='active', activated_at=now()`, idempotent via the unique index.
- [ ] 3.2 Test: running the backfill twice inserts no duplicate rows.
- [ ] 3.3 Run it against prod once (`DATABASE_URL` only); confirm row count matches the
      current YAML entry count.

## 4. `cmd/ingest` reads from `boards`

- [ ] 4.1 Add a DB-backed loader in `internal/ingest/sources` that queries `boards`
      filtered by provider and `status IN ('pending','active')`, returning
      `[]CompanyEntry` — the same shape the YAML loader produced.
- [ ] 4.2 Change `cmd/ingest`'s argument from a file path / `SOURCES_FILE` to a provider
      name; wire it to the new loader. Leave `--shard=i/n` behavior unchanged.
- [ ] 4.3 Test: `cmd/ingest <provider>` crawls exactly that provider's `pending`/`active`
      rows and excludes `rejected`/`retired` ones.
- [ ] 4.4 Integration test (`-tags=integration`): a shard selector still partitions one
      provider's companies round-robin with no company split across shards, now sourced
      from `boards` instead of a parsed file.

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
