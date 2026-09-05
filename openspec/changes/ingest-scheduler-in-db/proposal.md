## Why

The board catalog moved into Postgres (#2357) and `sources/` was retired (#2406), but the
SCHEDULE did not move with it. What decides that a provider is crawled — and how often, in
how many shards, under what timeout — is `deploy/bin/gen-ingest-timers.sh`, a bash script
that materialises 279 static systemd unit files.

The script does read `boards` now. The defect is that **nothing runs it.** Verified on prod
2026-09-04: no unit, no timer and no other script references it; it is invoked by hand over
ssh. `boards` changes continuously — a crowdsourced contribution activates a row on its
first successful crawl, `cmd/add-board` inserts one, a curator retires one — and between
manual runs the schedule is a stale photograph of the catalog. The Postgres-reading version
of the script reached the host at 03:49 on 2026-09-04 and had not been run since, so the
live timers were still a snapshot of the now-deleted `sources/` directory: named after
FILES, not provider keys.

That gap has already cost live crawls. Both found and fixed by hand during this change's
brainstorm:

- **`habr_career` was not being crawled at all.** The file was `sources/habrcareer.yml`, so
  the timer was `freehire-ingest@habrcareer`; the provider key in `sources.All` is
  `habr_career` — the only key of 186 that contains an underscore. While `cmd/ingest` took
  a file path this worked. After #2357 made the argument a provider name,
  `cmd/ingest habrcareer` matched zero boards and exited 0 — indistinguishable from a
  healthy empty provider. 784 live postings went stale. The first correct run ingested 675
  and closed 9 unseen.
- **`careerspage` had been running empty since 18 July.** Its boards were deliberately
  migrated to `manatal`; the timer was never retired. The script's own header claims a
  sweep retires such a timer — there is no sweep in the script, only a literal
  `systemctl disable` for three named providers.
- **`itechart` is a third zombie**, still live at the time of writing: a timer whose
  provider has no board in the catalog.

Two further defects of the same class are latent. The start minute is derived from a
provider's ALPHABETICAL POSITION (`min=$(( (i*41) % 60 ))`), so onboarding one provider
shifts the schedule of every provider after it — visible in the drift report as a
one-position cascade across `applitrack`, `apploi`, `arbeitnow`, `ashby`, `avature`. And
nothing in `deploy/` deploys itself: `./deploy/check-drift.sh` reports 190 differing files
and 30 host-only units against the current `main`.

The common root is that a provider's identity is written twice — once as a `boards.provider`
value, once as a systemd unit instance name — and nothing reconciles them. Every failure
above is a divergence between those two spellings, and every one of them is silent, because
a provider with no boards and a provider that is not scheduled both look like `exit 0`.

## What Changes

- **New `ingest_schedule` table** (curator-owned): per provider, the cadence, shard count,
  per-run timeout, an `enabled` flag with a mandatory `disabled_reason`, and a `notes`
  column carrying the measured reasoning that currently lives in the script's comments
  (paylocity's 10.42 s/board arithmetic, join's 1.5 req/s pacing, eightfold's proxy egress).
- **New `ingest_run_state` table** (machine-owned): per `(provider, shard)`, the due time,
  the claim, and the last run's outcome.
- **New `cmd/ingest-scheduler`**: a `Type=oneshot` worker run once a minute. It reads the
  live provider set from `boards`, LEFT JOINs `ingest_schedule` so a provider with no
  configuration row is scheduled on defaults, claims what is due with
  `FOR UPDATE SKIP LOCKED` under a concurrency cap, launches each due run as a transient
  unit via `systemd-run` carrying that row's own `TimeoutStartSec`, and exits. A claim
  outliving its timeout plus a grace window is reclaimed.
- **New `cmd/schedule-board`**: the control surface, shaped like the existing
  `cmd/add-board` — reports by default, writes only under `--apply`.
- **Shadow mode first**: `INGEST_SCHEDULER_APPLY` ships unset, so the first deployment
  computes and logs what it WOULD launch and launches nothing. The static timers keep
  running underneath until the shadow log has been read.
- **REMOVED once cut over**: the 279 generated unit files, `deploy/bin/gen-ingest-timers.sh`,
  and `deploy/bin/ingest-slot.sh` — the flock semaphore exists only because 279 independent
  timers cannot see each other, and one scheduler can simply count.
- Not in scope: the ~50 non-ingest cron workers. Their unit list is static and changes
  monthly, and several carry gates this change does not model (`skip-if-reindexing`, the
  two-file env split, advisory locks). systemd handles them; this change touches ingest only.

## Capabilities

### New Capabilities

- `ingest-scheduling`: what makes a provider's crawl due, how a due run is claimed and
  launched exactly once, how a stuck run is reclaimed, how fleet concurrency is bounded,
  and why a provider absent from the configuration table is scheduled on defaults rather
  than silently unscheduled.

### Modified Capabilities

None. `cmd/ingest` is unchanged — it still takes a provider name and an optional
`--shard=i/n`, exactly as `board-catalog-in-db`'s delta to `source-ingest` states. This
change replaces only what INVOKES it.

## Impact

- **Schema**: two new tables; no existing table altered. `boards` is read, never written.
- **New binaries**: `cmd/ingest-scheduler` and `cmd/schedule-board` must be added to
  `release.sh`'s worker-binary build list on the host, or the unit fails on a missing
  executable every minute.
- **New package**: `internal/ingest/ingestsched` — block `ingest`, and therefore a required
  entry in `internal/platform/arch/layering/blocks.go`, or the layering guard test fails.
- **Host**: 279 unit files collapse to one timer plus one scheduler service; the runs
  themselves are transient and leave no files. The scheduler unit runs as root so it may
  create transient units; each transient unit keeps `User=freehire`, so the crawl stays
  unprivileged.
- **Depends on** `board-catalog-in-db`, which is not yet archived. This change stacks on its
  `boards` table and its provider-name `cmd/ingest` contract; its §9 cleanup (`sources/`
  retirement) is already done as of #2406 and is not blocked by this change.
- **Docs**: `deploy/AGENTS.md` describes the per-provider timer fleet as the scheduling
  mechanism and must be rewritten; `internal/ingest/sources/AGENTS.md` gains the pointer to
  the new capability.
