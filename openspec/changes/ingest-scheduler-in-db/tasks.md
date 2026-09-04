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
- [x] 1.3 `internal/platform/db/queries/ingest_schedule.sql` + `make sqlc`: eligible
      providers (DISTINCT live `boards.provider` LEFT JOIN `ingest_schedule` — an INNER
      JOIN here would silently unschedule every unconfigured provider), run-state
      reconcile (`generate_series` upsert `ON CONFLICT DO NOTHING`, surplus-shard delete,
      departed-provider delete), the claim (`FOR UPDATE OF rs SKIP LOCKED`), and the
      finish write. The report read lands with `cmd/schedule-board` in §6.
- [x] 1.4 `managed` is documented as ROLLOUT-ONLY in its column comment, in
      `COMMENT ON TABLE ingest_schedule`, and at `Settings.Schedulable` — the one place it
      is read. Each names task 8.5 as its removal — this task first said 8.4, which is the
      cutover, not the removal. A rollout default that outlives its rollout restores the
      failure this change removes, so the pointer has to land on the right task.

## 2. `ingestsched` — eligibility and configuration

- [x] 2.1 New package `internal/ingest/ingestsched`, added to the block table in
      `internal/platform/arch/layering/blocks.go` (block `ingest`).
- [x] 2.2 `Effective(provider, *Override)` resolving cadence/shards/timeout from an
      optional override plus documented defaults (`DefaultShards`, `DefaultCadence`,
      `DefaultRunTimeout`), plus `Settings.ShardSelectors()` so "unsharded is shard 1 of 1"
      is stated once rather than in every caller.
      **Provenance is per-ROW, not per-field** — the task first said per-field, which the
      schema cannot support: the columns are NOT NULL with defaults, so an existing row
      always specifies every field, and inferring per-field provenance by comparing against
      the defaults would call an explicit hourly cadence a default by accident. Per-row is
      also exactly what the spec's report scenario asks for.
- [x] 2.3 Tests: an unconfigured provider resolves to the defaults and is marked
      `Overridden: false` and `Enabled: true` — the case the whole change turns on; an
      override supplies its values and carries its `Notes`; a disabled override carries its
      reason through to the report; shard selectors cover 1..n exactly once.
- [x] 2.4 `ValidateProviderKey`: shape check FIRST (lower-case ASCII, digits, `_`, `-` —
      an allowlist, since a denylist is a list of the attacks someone thought of), then the
      `sources.Taxonomy()` lookup. Rejects a space, `;`, `$(...)`, `/`, a newline, a
      case variant, and systemd's own `@` and `%`. Two tests earn their keep beyond the
      table: `habrcareer` is asserted to be UNKNOWN (the real production failure), and
      every key the live registry carries is asserted to pass — the test that catches a
      shape rule written to fit `greenhouse` and blind to `habr_career` or `whatjobs-br`.

## 3. `ingestsched` — run state

- [x] 3.1 `QueriesRepository.Eligible` and `.Reconcile`: materialise `(provider, 1..shards)`
      rows for every schedulable provider, delete surplus shards, and forget providers
      absent from the list — the sweep `gen-ingest-timers.sh` promised in its header and
      never had. Plus `Settings.Schedulable()`, so a disabled provider ends up with NO run
      state and the claim query needs no `enabled` predicate of its own.
- [x] 3.2 Integration tests: the roster comes from `boards` (a retired-only provider is not
      eligible); a provider with no override row resolves to defaults through the real
      LEFT JOIN; an override applies; a new provider gains its rows; raising the shard count
      leaves existing due times untouched (they are the fleet's stagger); lowering it drops
      only the surplus; a departed provider loses its rows; a disabled provider's state is
      deleted, not left idle at a months-old due time that would fire immediately.
- [x] 3.3 `Claim(limit, grace)`: due-or-reclaimable rows under `FOR UPDATE OF rs SKIP
      LOCKED`, `claimed_at` set, `next_due_at` advanced to `now() + cadence`. The timeout
      travels WITH the claim so a cadence edit landing between claim and launch cannot hand
      a run a budget the scheduler never decided on.
- [x] 3.4 Integration tests: eight concurrent claims against one due row take it exactly
      once; a claimed row is not re-claimed inside its window; a claim older than
      `timeout + grace` IS reclaimed; `next_due_at` advances from `now()`, so a six-hour
      outage owes one run and not six; the limit is respected exactly.
- [x] 3.5 `RecordFinish` writing the outcome and releasing the claim — proven to clear
      `claimed_at`, so the reclaim window guards genuinely stuck runs rather than every run
      that took a while.

## 4. The launcher port

- [x] 4.1 `Launcher` port + a recording fake. The port exists because `systemd-run` lives
      only on the crawl host, and the claim/due/reclaim logic must be testable without one.
- [x] 4.2 `SystemdLauncher`: `--unit` (named from the same provider string that selects the
      boards), `--collect` (or a failed run's leftover unit keeps its name and refuses the
      next launch — the fleet would stop one provider at a time, quietly),
      `--property=TimeoutStartSec` from the row, `CPUWeight`/`IOWeight` as the old units
      carried, `--uid` to the unprivileged account, and `EnvironmentFile`. An UNSHARDED
      provider gets no `--shard` at all rather than `1/1`: `cmd/ingest` already reads a
      missing selector as "crawl everything", and two spellings of one instruction is one
      more pair that can disagree.
- [x] 4.3 Test: a key refused by 2.4 never reaches the executor — asserted on the recorder
      having run nothing at all, not merely on the error.

## 5. `cmd/ingest-scheduler`

- [x] 5.1 `ingestsched.Scheduler.Tick` + `cmd/ingest-scheduler` (`worker.Main`/`Bootstrap`)
      and `config.LoadIngestScheduler`. Cap default 10 — `ingest-slot.sh`'s calibrated
      value, carried over unchanged so this change is about the MECHANISM and does not
      quietly re-tune throughput at the same time. A non-positive cap is floored to 1:
      a cap of 0 would read as saturated forever and stop the fleet with every check green.
- [x] 5.2 Shadow mode is the DEFAULT, backed by a separate `PreviewDueRuns` read. An
      integration test asserts preview and claim select the SAME rows with the same shard
      count and timeout — the predicate is written twice because sqlc cannot share one, and
      a divergence would make the shadow run a measurement of something else. A second test
      asserts preview moves no due time and claims nothing.
- [x] 5.3 A saturated tick claims NOTHING (limit 0, not a claim that is then discarded), so
      every due row stays claimable; it logs the saturation rather than exiting quietly.
- [x] 5.4 Tests over fakes: shadow launches nothing and previews; apply launches; saturated
      launches nothing and says so; a partially loaded fleet claims exactly `cap − in_flight`;
      a failed launch releases its claim at once (exit code 126) instead of idling the shard
      for the whole reclaim window; one failed launch does not stop the rest.
- [x] 5.5 Test: only `enabled && managed` providers are reconciled and launched.
      **Disabled and unmanaged are reported SEPARATELY** — a test drove this out: during
      cutover `Unmanaged` holds ~226 providers, and folding them into `Disabled` would bury
      the two that are genuinely turned off.

## 6. `cmd/schedule-board`

- [x] 6.1 Report mode (default): a table of every eligible provider — default vs override,
      cadence, timeout, state, next due, last finish, and the note or disable reason. The
      SHARDS column shows the intended count and, when they differ, what run state actually
      holds: an unreconciled shard-count change is exactly the drift this change removes, so
      it must be visible rather than averaged away.
- [x] 6.2 Write mode under `--apply`: `--shards/--cadence/--timeout/--notes`,
      `--disable --reason`, `--enable`, `--manage/--unmanage`. Every field is OPTIONAL and a
      missing one is left alone — a naive UPSERT would let raising the shard count silently
      reset a measured cadence. Contradictory flag pairs are refused, and so is a provider
      key the registry does not know: a typo written into the table would otherwise be
      reported as refused by every tick long after whoever typed it had moved on.
- [x] 6.3 Tests split as `cmd/add-board` does: `edit` (pure flag logic) unit-tested with no
      database; `Report`/`SaveOverride` integration-tested against a throwaway `testdb.Pool`
      — the default/override distinction, the partial-edit rule, the schema's refusal of an
      unexplained disable surfacing rather than being swallowed, and the last-run outcome
      surviving the round trip.

## 7. Deploy artifacts

- [x] 7.1 `deploy/systemd/freehire-ingest-scheduler.{service,timer}`. The unit records why
      it is the one privileged piece (it creates transient units; each carries
      `--uid=freehire`, so the crawl's own privilege is unchanged). The timer sets
      `Persistent=false` — unlike every per-provider timer it replaces — because catch-up
      now lives in `next_due_at`, and letting systemd ALSO replay a missed tick would add
      nothing and fire a burst on boot. No `RandomizedDelaySec` either: the stagger is the
      due time, not the timer.
- [x] 7.2 `ingest-scheduler` and `schedule-board` added to `release.sh`'s worker-binary
      build list, and to `.gitignore` (the repo has a guard test for the second).
      `release.sh` lives on the HOST; the repo copy is a record. Diffed the two while
      writing this (2026-09-04): the host carried `add-board` and
      `backfill-company-type-hint`, which the repo copy had never gained — both added here
      so the record is true, alongside this change's two.
- [x] 7.3 `internal/ingest/ingestsched/AGENTS.md`: the roster/override rule, the
      one-spelling rule with the `habr_career` incident behind it, the security argument for
      the shape-before-registry gate, the claim/reclaim contract, and a "measurements this
      inherits" section carrying every number from `gen-ingest-timers.sh`'s comments —
      paylocity's 10.42 s/board arithmetic, join's 1.5 req/s cumulative-budget finding,
      oracle's 6.82 s/board, eightfold's proxy, workstream's hydrating first pass, the
      twelve 3h providers' measurement, reed's quota, and why bayt/gulftalent are off.
- [x] 7.4 `AGENTS.md` (the module table), `deploy/AGENTS.md` and
      `internal/ingest/sources/AGENTS.md` updated; `check-doc-links` green at 289 links.

## 8. Cutover — OPERATOR STEPS, sequenced on prod

The commands, the order, and the rollback for each step are written out in
[runbook.md](runbook.md). It touches a live fleet of 238 providers over several days, and
the order matters more than any single command.

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

## 10. Review findings, folded back in

A code review of §1-§7 (2026-09-04) found one Critical and four Important. All are fixed
and each carries a test; recorded here because every one of them is a rule the next reader
needs, not a diff.

- [x] 10.1 CRITICAL — **nothing released a successful run's claim.** `RecordFinish` was
      called only when a LAUNCH failed; `cmd/ingest` knows nothing about the scheduler and a
      transient unit just ends. So `claimed_at` was set at claim and cleared by nothing:
      every successful run occupied a slot forever and the fleet saturated permanently after
      `Cap` launches, exit code 0, every check green. Added `Launcher.Finished` and
      `Scheduler.reap`, which runs BEFORE capacity is measured and in shadow mode too.
      Dropped `--collect` in the same change: systemd already collects a SUCCESSFUL
      transient unit, and that absence is how the reaper reads success — collecting failures
      too would erase the exit code and make "succeeded" and "failed" one answer.
- [x] 10.2 IMPORTANT — **run state was tracked only for MANAGED providers.** During the
      whole cutover `managed` defaults to false, so the departed-provider delete ran over
      the entire table every minute, and — worse — run state stayed empty, so the full day
      of shadow output §8.3 exists to read would have measured nothing. Tracking is now by
      `enabled`; `managed` gates the CLAIM, in SQL.
- [x] 10.3 IMPORTANT — **one unreconcilable provider stopped the whole tick**, against the
      package's own stated isolation rule. `Reconcile` now reports and steps over. `shards`
      gained an upper bound of 64 in both schema and CLI: unbounded, `--shards=100000000`
      makes `generate_series` outlive the scheduler's start timeout, and every following
      tick repeats it first.
- [x] 10.4 IMPORTANT — **the cap was a check-then-act pair**, so two overlapping
      invocations each read zero in flight and each claimed the whole cap. The tick now
      holds a Postgres advisory lock (`0x66687363`, registered in
      `internal/platform/migrate`). The flock semaphore this replaces WAS atomic.
- [x] 10.5 IMPORTANT — **during cutover the two ceilings cannot see each other**: the
      static units obey `ingest-slot.sh`'s 10 slots, the transient ones obey
      `INGEST_SCHEDULER_CAP`. 10 + 10 is 20 on a host calibrated for 10 — the I/O saturation
      that produced nginx 504s. The runbook now steps the scheduler's cap up with the share
      of the fleet it owns.
- [x] 10.6 Minor — both deletes now spare a CLAIMED row (erasing one loses the only record
      that a crawl is running); `GRACE_SECONDS=0` no longer silently means two minutes; the
      report says `not-managed` rather than asserting a static timer that §9 may already
      have deleted; the unit regains `After=meilisearch.service` for parity.

## 11. Review

- [x] 11.1 `gofmt -l` clean; `go vet ./...` clean; `go test ./...` clean.
- [x] 11.2 `go vet -tags=integration ./...` clean, and the FULL
      `go test -tags=integration ./...` across the whole module clean.
- [x] 11.3 `check-migrations` (squawk): 0 issues on the two added migrations — the
      `prefer-bigint-over-int` findings on shard/second/exit-code columns are suppressed
      with the reason beside each. `check-doc-links`: 289 links, all resolve. shellcheck
      clean on the changed `release.sh`.
- [ ] 11.4 Confirm on prod that every provider has completed a run within its cadence since
      cutover — the one measurement that proves the fleet did not quietly lose a provider,
      which is the failure this whole change exists to make impossible.
