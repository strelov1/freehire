# Ingest scheduling

## Scope
What decides that a provider's crawl is due, how a due run is claimed exactly once, how a
stuck run is reclaimed, and how the fleet's concurrency is bounded. Replaces
`deploy/bin/gen-ingest-timers.sh` and the ~279 static systemd units it materialised.

## Always true

- **The ROSTER is `boards`. `ingest_schedule` is a set of OVERRIDES.** The eligible
  providers are the distinct `boards.provider` values holding a `pending`/`active` row.
  `ingest_schedule` is joined LEFT, and a provider with no row is scheduled on the defaults
  in `settings.go`. An INNER JOIN here would silently unschedule every unconfigured
  provider — which is the exact failure the table was built to remove, so
  `TestEligibleDefaultsAProviderWithNoOverrideRow` asserts it through real SQL rather than
  trusting the query text.
- **Not crawling a provider must be written down.** `enabled = false` requires a
  `btrim`-non-empty `disabled_reason`, enforced by a CHECK on the table rather than only by
  `cmd/schedule-board`, because psql is a writer too. "Nobody configured it" and "we decided
  not to crawl it" must never be the same state.
- **There is ONE spelling of a provider key.** The unit name is built from the same string
  that selects the boards. This is the structural form of the `habr_career` incident: its
  timer was named `habrcareer` after the FILE `sources/habrcareer.yml`, the adapter answers
  to `habr_career`, and once `cmd/ingest` took a provider name instead of a path the run
  matched zero boards and exited 0 — indistinguishable from a healthy empty provider, for a
  day, over 784 live postings.
- **`ValidateProviderKey` is a security boundary, not a tidiness check.** Board rows can
  originate from crowdsourced submissions, and the key becomes both an argv element and a
  systemd unit name. The SHAPE check (lower-case ASCII, digits, `_`, `-`) runs FIRST and
  independently of the registry, because the registry is data that changes while "this
  cannot be a unit name" is a property of the string. It is an allowlist — a denylist is a
  list of the attacks someone thought of. `@` and `%` are excluded alongside the shell
  metacharacters: they are systemd's instance separator and specifier prefix, and either
  would produce a unit naming a different run than intended. The gate runs twice, in
  `Scheduler.Tick` (so a bad key never gains run state) and in `SystemdLauncher.Launch` (the
  last point where refusing is still possible). Neither alone is enough.
- **`next_due_at` advances at CLAIM, to `now() + cadence`.** At claim rather than at finish,
  so a 40-minute crawl cannot halve its own frequency. From `now()` rather than from the
  missed due time, so a six-hour outage owes exactly ONE run per provider instead of six
  that would stampede the fleet the moment the scheduler returns. That is systemd's
  `Persistent=true` behaviour reproduced deliberately, which is why the scheduler's own
  timer sets `Persistent=false`.
- **The shard COUNT comes from run state, not from the override.** `Reconcile` creates one
  row per shard, so the rows ARE the count. Reading `ingest_schedule.shards` instead is a
  second source for one fact, and an integration test caught them disagreeing (24 rows
  materialised, `Shards = 1` claimed) before the code ever ran.
- **A disabled provider's run state is DELETED, not left idle.** Keeping the row would leave
  a months-old `next_due_at` that fires the instant it is re-enabled, and it would force an
  `enabled` predicate into the claim query that it does not otherwise need.
- **A finished run tells nobody, so the scheduler must ASK.** `cmd/ingest` knows nothing
  about the scheduler, and a transient unit just ends. `Scheduler.reap` therefore queries
  the service manager about every claimed run before counting capacity. Without it,
  `claimed_at` would be set at claim and cleared by nothing: every successful run would
  occupy a slot forever and the fleet would saturate permanently after `Cap` launches — with
  every check green and exit code 0. That is why `Launch` does NOT pass `--collect`:
  systemd already collects a SUCCESSFUL transient unit, and that absence is how the reaper
  reads success, while a failed one lingers long enough for its exit code to be read.
- **During cutover there are TWO concurrency ceilings and they cannot see each other.** The
  static units go through `ingest-slot.sh`'s 10 flock slots; the scheduler's transient units
  bypass it and obey `INGEST_SCHEDULER_CAP`. A half-cut-over fleet can run 20 crawls on a
  host calibrated for 10 — the I/O saturation that produced nginx 504s. The runbook steps
  the scheduler's cap up in proportion to the share of providers it owns.
- **A saturated tick claims NOTHING and says so.** Not "claims and discards" — every due row
  stays claimable for the next tick, because advancing a due time under saturation would
  skip a cycle rather than defer it. `ingest-slot.sh` logged its skips for the same reason: a
  fleet that quietly stops crawling looks identical to a healthy one.
- **`managed` is ROLLOUT-ONLY.** While the static timers still run, only a managed provider
  is driven, so the two cannot both crawl one provider. This temporarily INVERTS the
  absence rule above, and task 8.5 of the change drops the column. A rollout default that
  outlives its rollout would restore the exact failure this package removes.

## How it works

`cmd/ingest-scheduler` runs once a minute (`Type=oneshot`, so it cannot stack on itself)
and calls `Scheduler.Tick`:

1. `Repo.Eligible` — the roster from `boards`, each provider resolved through `Effective`
   against its override or the defaults.
2. Partition: disabled (a decision), unmanaged (a rollout state — reported SEPARATELY,
   because during cutover it holds ~226 providers and would bury the two genuinely off),
   refused (a key the gate rejected), schedulable.
3. `Repo.Reconcile` — one run-state row per shard, surplus shards dropped, providers absent
   from the list forgotten. That last delete is the sweep `gen-ingest-timers.sh` promised in
   its header and never actually had.
4. `Repo.InFlight` and the cap decide the budget. `DefaultCap` is 10 — `ingest-slot.sh`'s
   calibrated value, carried over unchanged so this change is about the mechanism and does
   not re-tune throughput at the same time.
5. Shadow (default) → `Repo.PreviewDue` reports and mutates nothing.
   Apply → `Repo.Claim` under `FOR UPDATE ... SKIP LOCKED`, then `Launcher.Launch` per run.
6. A failed launch is recorded with exit code 126 and its claim released at once, rather
   than idling that shard for the whole reclaim window over an error already known.

A run becomes a TRANSIENT systemd unit (`systemd-run`), not an instance of a static
template, because the per-run timeout varies (3000s generally, 4500s for the
per-posting-detail providers) and a template carries one value. `--collect` is load-bearing:
without it a failed run's unit keeps its name and the next launch is refused, so the fleet
would stop one provider at a time with nothing failing loudly.

## The measurements this inherits

These numbers came from `gen-ingest-timers.sh`'s comments and are the reason the overrides
are what they are. A number that outlives its measurement becomes folklore, so they live in
`ingest_schedule.notes` as well as here.

- **paylocity — 24 shards, 4500s.** ~9,477 boards. Measured ~10.42 s/board on the unsharded
  run (288 boards in 3000s). 395 boards per shard × 10.42 s ≈ 4,117 s, which is why the
  timeout is raised past the generic 3000s rather than the shard count cut to fit it. Before
  sharding, a run reached ~3% of the file and — crawl order being fixed with no resume
  cursor — hit the same leading slice every cycle while 74% of its live jobs sat unswept.
- **workday — 6 shards.** ~6,165 boards, 429-throttled too hard to finish in one run.
- **oracle — 4 shards, 4500s.** 796 boards with a per-posting detail fan-out, ~6.82 s/board.
  Unsharded it reached ~35% before the timeout; 18% of its live jobs stayed unswept for days.
- **join — 5 shards.** Metered by REQUEST RATE, not concurrency. Raised from 4 to 5 alongside
  a pace drop to 1.5 req/s: refusals accelerated through a long run at 2 req/s, which reads
  as a cumulative budget on top of the per-second one. At 1.5 req/s a shard is ~35 min of
  requests, which only a 5-way split keeps under the timeout.
- **eightfold — 4 shards.** Egresses through the residential proxy (its edge IP-blocklists
  the prod address). A few boards are enormous (nvidia/hp/citi), so sharding isolates them
  and staggering keeps the shards off each other's single proxy IP.
- **dayforce — 4 shards, 4500s**; **workstream — 2 shards, 4500s** (paced to ~0.5 req/s by
  its origin; the FIRST crawl hydrates every posting, ~4,700 requests over two hours, and
  will hit the timeout a couple of times — expected and safe, since a timed-out run still
  persists what it hydrated).
- **The twelve 3h providers** (bamboohr, icims, paycom, gupy, mycareersfuture, ukg,
  careerplug, jibe, jazzhr, vagas, apple, taleo): measured 2026-07-31 to average ≥25 min per
  run and together 65% of all ingest busy-time. Crawled hourly they never finished a sweep.
- **reed — 6h.** A per-hour API request quota its full crawl blows past (403 "exceeded your
  per-hour request limit").
- **bayt, gulftalent — disabled.** Both egress via the Chrome-fingerprint client, which has
  no proxy support, and both hard-403 the prod datacenter IP. An hourly timer only churned
  403s and `board_health` noise. Re-enable once proxy support is wired for that client.
