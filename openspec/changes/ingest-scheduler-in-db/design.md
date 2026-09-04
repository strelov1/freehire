## Context

`cmd/ingest` takes a provider name and reads that provider's boards from Postgres
(#2357). What INVOKES it is `deploy/bin/gen-ingest-timers.sh`, which materialises 279
static systemd units — 238 per-provider timers, 45 shard timers, and the templates behind
them — from a `SELECT provider FROM boards` plus a set of bash constants.

The script is correct and well-reasoned. Its problem is that it is a MATERIALISER run by
hand: nothing on the host invokes it (verified 2026-09-04 — no unit, no timer, no script
references it). Between runs the unit files are a photograph of a catalog that has since
moved, and every divergence between the photograph and the catalog is silent, because a
provider with no boards and a provider with no timer both produce `exit 0`.

Constraints this design inherits:

- **`cmd/ingest` must not change.** It is proven, and its contract is already specified.
- **The reasoning in the script's comments is load-bearing** (paylocity's 10.42 s/board
  arithmetic, join's 1.5 req/s cumulative-budget finding, eightfold's proxy egress,
  workstream's first-pass hydration overrun). It is measurement, not preference, and must
  survive the move rather than evaporate into column values.
- **The fleet is live.** 238 providers crawl continuously; a cutover that double-runs or
  stops a provider is a catalogue-freshness incident, not a rollback.
- **`deploy/` does not deploy itself**, and currently reports 190 differing files against
  the host. The cutover must be sequenced against the host's real state, not against `deploy/`.

## Goals / Non-Goals

**Goals:**

- One spelling of a provider key: the one in `boards`. No second copy in a filename.
- A provider is scheduled by DEFAULT; not scheduling it requires an explicit, reasoned row.
- The schedule reflects the catalog within a minute, with no human in the loop.
- Preserve what the current fleet does well: per-provider isolation, per-provider timeout,
  sharding, cgroup accounting, catch-up after downtime, and a hard concurrency ceiling.
- Keep the measured reasoning attached to the numbers it justifies.

**Non-Goals:**

- The ~50 non-ingest cron workers. Their roster is static; several carry gates this design
  does not model (`skip-if-reindexing`, the two-file env split, Postgres advisory locks).
- Per-BOARD cadence. The unit of scheduling stays the provider (or its shard), as today.
  Deriving a board's cadence from `board_health` is a real opportunity and a separate change.
- A web admin surface. The control surface is a CLI worker, matching `cmd/add-board`.
- Replacing systemd as the executor. systemd still runs the crawl; it stops being the
  place the schedule is WRITTEN.

## Decisions

### Two tables, split by who owns the rows

`ingest_schedule` is curator-owned (`cadence_sec`, `shards`, `timeout_sec`, `enabled`,
`disabled_reason`, `notes`). `ingest_run_state` is machine-owned (`next_due_at`,
`claimed_at`, `last_started_at`, `last_finished_at`, `last_exit_code`, `last_error`),
keyed `(provider, shard)`.

The cadence is stored in SECONDS rather than as a Postgres `interval`. sqlc maps `interval`
to `pgtype.Interval`, and that type would then travel through every caller and every test
for no expressive gain; `cmd/schedule-board` accepts `--cadence=3h` and stores 10800. The
`> 0` CHECK is also plain on an integer, where on an interval it is not.

This mirrors the split the repository already makes between `boards` (the catalog) and
`board_health` (runtime state), and it keeps a curator's decision from being overwritten by
a run, and a run's outcome from being lost when a curator edits a cadence.

*Alternative considered:* one table with both concerns. Rejected — every schedule edit
would contend with the claim UPDATE that fires every minute, and a careless `UPSERT` from
the CLI would clobber run state.

### Configuration is a set of OVERRIDES, not the roster

The eligible provider set comes from `boards`; `ingest_schedule` is `LEFT JOIN`ed. A
provider with no row runs on defaults (1 shard, hourly, default timeout).

This is the decision that closes the defect class. If the configuration table were the
roster, then "nobody added a row" and "we decided not to crawl this" would be the same
state — which is exactly the failure `habr_career` and `careerspage` were: a provider
missing from, or stale in, a second list. Making absence mean *scheduled on defaults*
forces every non-crawl to be written down with a reason.

*Alternative considered:* materialise one row per provider at insert time and treat the
table as the roster. Rejected for the reason above; a materialising step that must be run
is the very thing being removed.

### The scheduler is a one-shot worker on a one-minute timer, not a daemon

`cmd/ingest-scheduler` starts, claims what is due, launches it, and exits — the shape of
every other worker in `cmd/`.

- `Type=oneshot` will not start a second instance while the first is active, so the
  scheduler cannot stack on itself.
- A crash costs one minute, not the fleet — the next tick recovers, because all state is in
  Postgres.
- It fits `worker.Main`/`Bootstrap` and the existing heartbeat and exit-code conventions,
  so it needs no new operational vocabulary.

*Alternative considered:* a long-lived `Restart=always` daemon holding an in-memory
schedule. Rejected — it introduces a single point of failure the current 279 independent
timers do not have, and it makes the schedule's live state unreadable except through the
process.

### Runs are launched as transient units via `systemd-run`

Each claimed run becomes `systemd-run --unit=freehire-ingest-run-<provider>-<shard>
--property=TimeoutStartSec=<row's timeout> --uid=freehire … ingest <provider> --shard=i/n`.

- The per-run timeout varies (3000 s generally, 4500 s for paylocity/dayforce/workstream);
  a static unit template can carry only one value, so per-run properties are the point.
- The transient unit outlives the scheduler invocation, so the scheduler does not hold its
  children and a slow crawl cannot extend the scheduler's own runtime.
- cgroup accounting, `CPUWeight`/`IOWeight`, and journal capture stay exactly as they are
  today. The crawl keeps running as `freehire`; only the scheduler is privileged.

*Alternatives considered:* (a) forking `ingest` as a child and waiting — reintroduces
stacking, since the scheduler's own runtime becomes the longest crawl; (b) `systemctl start`
on a static template — cannot vary the timeout per run.

**Security note, and a hard requirement of this decision:** the scheduler builds an argv
from `boards.provider`, and board rows can originate from crowdsourced submissions. The
provider key SHALL be validated against the adapter registry (`sources.Taxonomy()`) and a
strict character class before it can reach `systemd-run` or a unit name. A provider that
does not resolve to a registered adapter is reported and skipped, never launched.

### The launcher is a port, not a direct exec

`systemd-run` is reachable only on the production host. The scheduler takes a `Launcher`
interface; the systemd implementation is one adapter, and tests use a recording fake. This
keeps the claim/due/reclaim logic — the part worth testing — free of the host.

### The due time advances at CLAIM, on the wall clock

`next_due_at = now() + cadence`, set in the claiming transaction.

- Advancing at claim rather than at finish stops a 40-minute crawl from silently halving
  its own frequency.
- Using `now()` rather than `next_due_at + cadence` caps catch-up at ONE run: after a
  six-hour outage a provider runs once and returns to cadence, instead of owing six runs
  that would stampede the fleet the moment the scheduler comes back. This deliberately
  differs from systemd's `Persistent=true`, which would also fire only once but for a
  different reason; the behaviour matches, the mechanism is explicit.

### Concurrency is a `LIMIT`, not a flock

`ingest-slot.sh` exists because 279 independent timers cannot see each other. One
scheduler can count: in-flight runs are rows with a live claim, and a tick launches at most
`cap − in_flight`. The cap starts at 10 — the value `ingest-slot.sh` was calibrated to
against this fleet, so the change does not silently re-tune throughput while it is also
changing the mechanism.

A saturated tick logs that it skipped. Leaving the row due (rather than advancing it) is
what makes a skip harmless: the next tick picks it up.

### A rollout-only `managed` flag, with its removal in the task list

During cutover the static timers and the scheduler must not both drive a provider. The
scheduler therefore launches only providers whose `ingest_schedule.managed` is true, and
the cutover flips one provider at a time, disabling its static timer in the same step.

This temporarily INVERTS the "absence means scheduled" rule above, which is a real hazard:
a rollout default that quietly becomes permanent would restore the exact failure mode this
change removes. The column is therefore dropped in a numbered task once every provider is
managed, and that task is not optional.

### Package placement

`internal/ingest/ingestsched`, block `ingest` — it reads `boardcatalog` and knows about
providers. It must be added to `internal/platform/arch/layering/blocks.go` or the layering
guard fails.

### The measured reasoning moves into `notes`, and the AGENTS.md keeps the argument

Each override row carries a `notes` value stating what was measured. The narrative — why
paylocity is 24 shards and not 4, why join is 5 — moves into the capability's AGENTS.md
section. A number without its measurement is how a calibrated fleet decays into folklore.

## Risks / Trade-offs

- **Command construction from a crowdsourced value** → validate the provider key against
  the adapter registry and a strict character class before it reaches `systemd-run`;
  a key that does not resolve is reported, never launched. Covered by a requirement, not
  only by review.
- **Double-running during cutover** → the `managed` flag plus per-provider sequencing;
  disabling the static timer and flipping the flag is one step, and the rollback is the
  reverse of that step.
- **Thundering herd after a long scheduler outage** → catch-up is capped at one run per
  provider by advancing from `now()`, and the concurrency cap bounds the tick regardless.
- **The scheduler runs as root** → it is the only privileged piece; the transient units it
  creates specify the unprivileged account, so the crawl's privilege is unchanged from today.
- **`systemd-run` is host-only** → the `Launcher` port keeps the schedulable logic testable
  without systemd; a non-systemd environment is a missing adapter, not a broken worker.
- **`deploy/` is already 190 files adrift** → the cutover reads the HOST's enabled units as
  the source of truth for what to disable, and `deploy/` is reconciled as a numbered task
  rather than assumed correct.
- **Losing per-provider isolation if the scheduler wedges** → a wedged scheduler stops all
  crawling, where 279 timers would degrade one provider at a time. Mitigated by `oneshot`
  (no stacking), by all state living in Postgres (no in-memory recovery), and by a
  freshness gauge in `cmd/queue-metrics` that alerts when providers stop completing runs.

## Migration Plan

1. **Ship inert.** Tables, `ingestsched`, `cmd/ingest-scheduler`, `cmd/schedule-board`.
   Add both binaries to `release.sh`'s build list. The scheduler's timer is installed with
   launching disabled — shadow mode is the default, so the deployment cannot touch the fleet.
2. **Seed the overrides** from the script's constants: the twelve `HEAVY` providers' 3 h
   cadence, `reed`'s 6 h, the seven shard families' counts and raised timeouts, and
   `bayt`/`gulftalent` disabled with their stated reason. Each with its `notes`.
3. **Read a day of shadow output.** Compare what the scheduler would have launched against
   what the timers actually launched. Discrepancies here are cheap; after step 4 they are not.
4. **Cut over provider by provider**, unsharded first: disable the static timer, flip
   `managed`. Roll back by reversing the pair.
5. **Cut over the seven shard families** last — they carry the raised timeouts and the
   tightest pacing, so they are the ones where a mistake costs the most.
6. **Retire** `gen-ingest-timers.sh` and `ingest-slot.sh`, delete the generated units, drop
   the `managed` column, and reconcile `deploy/` against the host.

Rollback at any point before step 6 is: re-enable the static timers (they are still on the
host and still correct) and turn launching off. The tables can stay; they are inert without it.

## Open Questions

- **The reclaim grace window.** A claim is dead at `timeout + grace`. The value should be
  large enough that a run systemd is still tearing down is not relaunched, and small enough
  that a crashed scheduler does not idle a provider for a cycle. To be fixed with a measured
  value during the shadow run rather than guessed now.
- **Whether `queue-metrics` gains the freshness gauge in this change or the next.** The
  gauge is what makes a wedged scheduler loud; the argument for folding it in here is that
  this change is what concentrates the failure into one process.
