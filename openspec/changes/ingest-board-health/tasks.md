## 1. Schema + generated queries

- [x] 1.1 Add a migration creating `board_health` (PK `(provider, board)`;
  `consecutive_failures`, `cooldown_until`, `last_error`, `last_error_at`,
  `last_success_at`, `last_ingested_count`, `last_run_at`) per design D1.
- [x] 1.2 Add sqlc queries: `GetBoardCooldown` (by provider+board), `RecordBoardSuccess`
  (reset failures, clear cooldown, stamp success + count), `RecordBoardFailure` (increment
  failures, store error, set cooldown), `ListUnhealthyBoards` (failures>0 or cooled).
  Run `make sqlc` and commit generated code.

## 2. Backoff policy (pure, DB-free)

- [x] 2.1 Implement the backoff function `cooldownFor(consecutiveFailures int) (time.Duration, bool)`
  in `internal/pipeline` (threshold 3, `6h * 2^(f-3)` capped at 24h). Unit-test the whole
  table incl. below-threshold (no cooldown), f=3→6h, f=5→24h, f=10→24h (capped).

## 3. Runner: cooldown-skip + outcome recording

- [x] 3.1 Add the optional `BoardHealth` port (Cooldown / RecordSuccess / RecordFailure)
  and a `Cooled` field on `RunStats`/`Stats`; a nil port keeps today's behavior.
- [x] 3.2 In `ingestBoard`: check `Cooldown` BEFORE the adapter lookup — if cooled, log
  once, count `Cooled`, return without crawling; else crawl and `RecordSuccess`/`RecordFailure`
  by board-level outcome (unknown provider / fetch error = failure; per design D4). Handle
  the streaming path's board-level outcome (confirm the zero-progress question from design).
- [x] 3.3 Existing Runner tests stay green with a nil port; add tests with a fake
  `BoardHealth`: a cooled board is not fetched (adapter Fetch not called) and counted
  `Cooled`; a failing board records failure; a success clears prior failure state.

## 4. cmd/ingest wiring + visibility

- [x] 4.1 Implement `QueriesBoardHealth` over `*db.Queries` in `cmd/ingest`, applying the
  backoff function in `RecordFailure`; inject it into the Runner.
- [x] 4.2 Emit a per-run summary log of unhealthy boards (from `ListUnhealthyBoards` or the
  run's accumulated outcomes) — names, last error, next-eligible time — distinct from the
  routine per-board logs.
- [x] 4.3 Integration test (`//go:build integration`, testcontainer): a board that fails 3×
  gets a cooldown and is skipped on the next run; a subsequent success clears it.

## 5. Ship

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` (+ integration tag) green.
- [x] 5.2 Document the manual migration step: apply the `board_health` migration to prod
  PG18 BEFORE deploying the new binary (no versioned runner). Note the `board_health`
  query for operators ("which boards are failing / cooled").
- [x] 5.3 Update `AGENT.md`: add the board-health/cooldown convention to the ingest section
  (runtime-state sidecar; YAML remains the catalog + cadence source of truth).
