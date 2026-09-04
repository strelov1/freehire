## 1. Schema

- [x] 1.1 Migration `0132_ingest_schedule.sql`: `provider` PK, `shards` int DEFAULT 1
      CHECK > 0, `cadence_sec` int DEFAULT 3600 CHECK > 0, `timeout_sec` int DEFAULT 3000
      CHECK > 0, `enabled` bool DEFAULT true, `disabled_reason` text, `notes` text,
      `managed` bool DEFAULT false, `created_at`, `updated_at`. CHECK
      `ingest_schedule_disabled_needs_reason`: `enabled` false REQUIRES a `btrim`-non-empty
      `disabled_reason` — the spec's "absence and decision are not the same state" rule
      belongs in the schema, not only in the writer, because psql is a writer too.
      Cadence is SECONDS, not `interval`: sqlc maps `interval` to `pgtype.Interval`, which
      would travel through every caller for no gain (recorded in design.md).
- [x] 1.2 Migration `0133_ingest_run_state.sql`: `(provider, shard)` PK, `shard` CHECK > 0,
      `next_due_at` timestamptz NOT NULL, `claimed_at`, `last_started_at`,
      `last_finished_at`, `last_exit_code` int, `last_error` text. Partial index on
      `(next_due_at) WHERE claimed_at IS NULL` (the claim predicate) and on
      `(claimed_at) WHERE claimed_at IS NOT NULL` (the reclaim walk).
      Proven by `internal/ingest/ingestsched/schema_integration_test.go`, which asserts the
      SQLSTATE rather than "some error" — a schema test that accepts any error passes
      identically when the table is missing.
- [ ] 1.3 Add sqlc queries for: eligible providers (`boards` GROUP BY provider, live
      statuses, LEFT JOIN `ingest_schedule`), run-state reconcile (upsert the
      `(provider, 1..shards)` rows, delete rows beyond `shards`), the claim
      (`FOR UPDATE SKIP LOCKED` with the reclaim window), the finish write, and the report
      read. Run `make sqlc`.
- [ ] 1.4 Document `managed` in the migration's COMMENT as ROLLOUT-ONLY, naming task 8.4 as
      its removal — a rollout default that outlives its rollout restores the failure this
      change removes.

## 2. `ingestsched` — eligibility and configuration

- [ ] 2.1 New package `internal/ingest/ingestsched`; add it to the block table in
      `internal/platform/arch/layering/blocks.go` (block `ingest`) or the layering guard
      test fails.
- [ ] 2.2 `Effective(provider)` resolving a provider's effective cadence/shards/timeout
      from an optional override row plus documented defaults, reporting for each field
      whether it came from a default or an override.
- [ ] 2.3 Tests: an unconfigured eligible provider resolves to hourly / 1 shard / default
      timeout and is marked as defaulted; an override wins per-field; a row with `enabled`
      false is excluded and carries its reason.
- [ ] 2.4 `ValidateProviderKey`: a provider key SHALL resolve in `sources.Taxonomy()` AND
      match a strict character class before it may reach a unit name or an argv. Tests
      cover an unregistered key, a key with a shell metacharacter, and a key with a space —
      each reported and skipped, never launched.

## 3. `ingestsched` — run state

- [ ] 3.1 `Reconcile`: materialise `(provider, 1..shards)` run-state rows for every
      eligible provider, delete rows beyond the current shard count, seed `next_due_at` for
      a new row.
- [ ] 3.2 Integration tests (real Postgres, testcontainers): a new provider gains its rows;
      raising the shard count adds only the new shards and leaves existing due times
      untouched; lowering it deletes only the surplus; a provider whose boards all became
      `retired` loses its rows.
- [ ] 3.3 `Claim(limit)`: select due, unclaimed-or-reclaimable rows under
      `FOR UPDATE SKIP LOCKED`, set `claimed_at`, advance `next_due_at = now() + cadence`.
- [ ] 3.4 Integration tests: two concurrent claims never return the same row; a claimed row
      is not re-claimed inside its window; a claim older than `timeout + grace` IS
      reclaimed; `next_due_at` advances from `now()` so a six-hour outage owes exactly one
      run, not six.
- [ ] 3.5 `RecordFinish(provider, shard, exitCode, err)` writing the outcome.

## 4. The launcher port

- [ ] 4.1 Define `Launcher` (one method: launch a validated run with its provider, shard
      selector and timeout) and a recording fake for tests.
- [ ] 4.2 `systemdLauncher`: `systemd-run` with `--unit`, `--property=TimeoutStartSec=`,
      the unprivileged account, and the `ingest <provider> [--shard=i/n]` argv. Unit tests
      assert the constructed argument list, including that the per-provider timeout — not
      the default — reaches it.
- [ ] 4.3 Test: a provider key rejected by 2.4 never reaches the `Launcher` at all.

## 5. `cmd/ingest-scheduler`

- [ ] 5.1 The worker: `worker.Bootstrap`, reconcile, claim up to `cap − in_flight`, launch,
      exit. Concurrency cap default 10 (`ingest-slot.sh`'s calibrated value), overridable.
- [ ] 5.2 Shadow mode is the DEFAULT: without `INGEST_SCHEDULER_APPLY` it resolves,
      reports every decision, launches nothing and advances no due time.
- [ ] 5.3 A saturated tick logs that it skipped and leaves every due row claimable.
- [ ] 5.4 Tests over the fake launcher: shadow mode launches nothing and mutates no state;
      apply mode launches; a saturated fleet launches nothing and says so; a partially
      loaded fleet launches exactly the free capacity.
- [ ] 5.5 Test: only `managed` providers are launched while the column exists.

## 6. `cmd/schedule-board`

- [ ] 6.1 Report mode (default): every eligible provider with effective cadence, shards,
      timeout, default-vs-override per field, `managed`, and last run outcome.
- [ ] 6.2 Write mode under `--apply`: set cadence / shards / timeout / notes; `--disable
      --reason=…`; `--manage` to flip `managed`. Refuse a disable with no reason at the CLI
      as well as in the schema.
- [ ] 6.3 Tests mirroring `cmd/add-board`'s split: the DB-touching cores tested against a
      throwaway `testdb.Pool`, the dry-run and invalid-input paths on the CLI wrappers.

## 7. Deploy artifacts

- [ ] 7.1 `deploy/systemd/freehire-ingest-scheduler.service` (+ `.timer`, `OnCalendar=*:*:00`).
      The scheduler runs privileged so it may create transient units; record why in the unit.
- [ ] 7.2 Add `ingest-scheduler` and `schedule-board` to `release.sh`'s worker-binary build
      list — a worker whose binary was never built fails every minute.
- [ ] 7.3 Write `internal/ingest/ingestsched/AGENTS.md`: the eligibility rule, the
      absence-means-scheduled rule and why, the claim/reclaim contract, the validation gate
      before `systemd-run`, and the MEASURED reasoning inherited from
      `gen-ingest-timers.sh` (paylocity's 10.42 s/board, join's 1.5 req/s, eightfold's
      proxy, workstream's hydrating first pass).
- [ ] 7.4 Update `deploy/AGENTS.md` and `internal/ingest/sources/AGENTS.md` to point at the
      new capability; `pnpm check:links` must stay green.

## 8. Cutover — OPERATOR STEPS, sequenced on prod

- [ ] 8.1 Deploy §1-§7 with shadow mode on. Confirm the unit runs, reports, and launches
      nothing.
- [ ] 8.2 Seed the overrides from `gen-ingest-timers.sh`'s constants, each with its `notes`:
      the twelve `HEAVY` providers at 3 h; `reed` at 6 h; the seven shard families
      (workday 6, eightfold 4, oracle 4, paylocity 24, join 5, dayforce 4, workstream 2)
      with their raised timeouts; `bayt`/`gulftalent` disabled with the fingerprint-client
      reason. Also retire the two live zombies found during design (`itechart`, and
      `custom`'s leftover unit file).
- [ ] 8.3 Read a full day of shadow output against what the timers actually launched.
      Resolve every discrepancy BEFORE any provider is cut over.
- [ ] 8.4 Cut over the unsharded providers in waves, then the seven shard families last.
      Each provider: `systemctl disable --now freehire-ingest@<p>.timer` and flip `managed`
      as one step. Read the HOST's enabled units as the source of truth for what to
      disable — `deploy/` is 190 files adrift and must not drive this.
- [ ] 8.5 Once every provider is managed: drop the `managed` column and its gate, so
      absence once again means scheduled on defaults. NOT OPTIONAL — see 1.4.

## 9. Retire the old path

- [ ] 9.1 Delete the generated unit files from the host and from `deploy/systemd`.
- [ ] 9.2 Delete `deploy/bin/gen-ingest-timers.sh` and `deploy/bin/ingest-slot.sh`.
- [ ] 9.3 Reconcile `deploy/` against the host and confirm `./deploy/check-drift.sh` exits 0.

## 10. Review

- [ ] 10.1 `gofmt -l` clean; `go vet ./...`; `go test ./...`.
- [ ] 10.2 `go vet -tags=integration ./...` and the full `go test -tags=integration ./...`.
- [ ] 10.3 `pnpm check:sql` on the added migrations; `pnpm check:links`; shellcheck via the
      `artifacts` job on any changed `*.sh`.
- [ ] 10.4 Confirm on prod that every provider has completed a run within its cadence since
      cutover — the one measurement that proves the fleet did not quietly lose a provider,
      which is the failure this whole change exists to make impossible.
