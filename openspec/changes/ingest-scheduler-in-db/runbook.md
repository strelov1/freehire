# Cutover runbook

The operator half of `openspec/changes/ingest-scheduler-in-db` — §8 and §9 of `tasks.md`.
Written out because it touches a live fleet of 238 providers over several days, and the
order matters more than any single command.

**Host:** `root@89.167.94.146`. Every command below runs there unless it says otherwise.

**The one rule that outranks the rest:** during cutover a provider is driven either by its
static timer or by the scheduler, never both. `ingest_schedule.managed` is what decides,
and the disable-the-timer and flip-the-flag steps are ONE step. If you do only half, you
either double-crawl a provider or stop crawling it — and stopping is the silent one.

**The second rule, which is easy to miss:** during cutover there are TWO concurrency
ceilings, and they cannot see each other. The static units go through
`/opt/freehire/bin/ingest-slot.sh` and its 10 flock slots; the scheduler's transient units
bypass that script entirely and obey `INGEST_SCHEDULER_CAP`. A half-cut-over fleet can
therefore run 20 crawls at once on a host calibrated for 10 — which is the I/O saturation
that produced nginx 504s the last time it happened. So:

**Set the scheduler's cap in proportion to how much of the fleet it owns.** Start at 2,
and raise it roughly in step with the share of providers cut over, reaching 10 only when
`ingest-slot.sh` is gone (§9). `INGEST_SCHEDULER_CAP` lives in `/opt/freehire/.env`.

```
# a rough guide, not a formula — measure /proc/pressure/io rather than trusting it
#   first wave (~3 providers)   INGEST_SCHEDULER_CAP=2
#   ~25% cut over              INGEST_SCHEDULER_CAP=3
#   ~50%                       INGEST_SCHEDULER_CAP=5
#   ~90%                       INGEST_SCHEDULER_CAP=8
#   after §9                   INGEST_SCHEDULER_CAP=10   (or unset; 10 is the default)
```

---

## Before anything

Disarm the autodeploy poll, so the code deploy and the unit changes land in one controlled
window instead of racing a 10-minute timer. (This is what the board-catalog cutover had to
do mid-flight after discovering it was armed.)

```
systemctl stop freehire-autodeploy.timer
```

Re-arm it at the very end.

## 8.1 — Deploy, in shadow

```
/opt/freehire/bin/release.sh freehire
```

`release.sh` on the host must build `ingest-scheduler` and `schedule-board`. The repo copy
of its build list now names them; **the host copy is a separate file and is what actually
runs** — check it before releasing:

```
grep -c ingest-scheduler /opt/freehire/bin/release.sh   # must be 1
```

Then install the scheduler's unit and timer from `deploy/systemd/`, `daemon-reload`, and
start the timer. `INGEST_SCHEDULER_APPLY` stays UNSET — shadow is the default, and a
scheduler that launched on install would double-crawl every provider at once.

Confirm one tick:

```
systemctl start freehire-ingest-scheduler.service
journalctl -u freehire-ingest-scheduler.service -n 40 --no-pager -o cat
```

Expect `mode=shadow`, `eligible=238`, `scheduled=0`, `launched=0`, a `would_launch` count,
and one line naming the 238 providers still owned by their static timer. **`launched=0` is
the thing to verify.** If `eligible=0` the tick fails loudly by design — that means the
catalog read is wrong, not that the catalogue is empty.

## 8.2 — Seed the overrides

Every number below is a MEASUREMENT; the `--notes` are not decoration. The full reasoning
is in `internal/ingest/ingestsched/AGENTS.md`. Run each without `--apply` first.

```
schedule-board --provider=paylocity  --shards=24 --cadence=24h --timeout=4500s --apply \
  --notes="~10.42s/board measured; 395 boards/shard = ~4117s, so 4500s not 3000s"
schedule-board --provider=workday    --shards=6  --cadence=6h  --apply \
  --notes="~6165 boards, 429-throttled; one shard per hour covers the file in 6h"
schedule-board --provider=oracle     --shards=4  --cadence=4h  --timeout=4500s --apply \
  --notes="796 boards, per-posting detail fan-out, ~6.82s/board"
schedule-board --provider=join       --shards=5  --cadence=5h  --apply \
  --notes="metered by request RATE; 1.5 req/s, ~35min/shard — 4 shards did not fit"
schedule-board --provider=eightfold  --shards=4  --cadence=4h  --apply \
  --notes="residential proxy egress; nvidia/hp/citi are huge, stagger keeps them off one IP"
schedule-board --provider=dayforce   --shards=4  --cadence=4h  --timeout=4500s --apply
schedule-board --provider=workstream --shards=2  --cadence=6h  --timeout=4500s --apply \
  --notes="paced ~0.5 req/s by origin; first crawl hydrates everything, ~2h, will time out twice"
```

The twelve 3h providers — measured 2026-07-31 at ≥25 min/run and together 65% of all ingest
busy-time:

```
for p in bamboohr icims paycom gupy mycareersfuture ukg careerplug jibe jazzhr vagas apple taleo; do
  schedule-board --provider=$p --cadence=3h --apply \
    --notes="measured >=25min/run 2026-07-31; hourly never finished a sweep"
done
schedule-board --provider=reed --cadence=6h --apply \
  --notes="per-hour API request quota; a full crawl 403s past it"
```

The two that must not crawl:

```
schedule-board --provider=bayt --disable --apply \
  --reason="fingerprint client has no proxy support; hard-403s the prod datacenter IP"
schedule-board --provider=gulftalent --disable --apply \
  --reason="fingerprint client has no proxy support; hard-403s the prod datacenter IP"
```

Everything else is left with NO row on purpose — hourly, unsharded, 3000s is what those
~220 providers already run on, and a row that only restates a default is a value that can
drift from it later.

Also retire the two zombies found while designing this. `custom` never named a real
provider; `itechart` has no board in the catalogue:

```
systemctl disable --now freehire-ingest@custom.timer
rm -f /etc/systemd/system/freehire-ingest@custom.timer
systemctl disable --now freehire-ingest@itechart.timer
rm -f /etc/systemd/system/freehire-ingest@itechart.timer
```

Read the result back:

```
schedule-board            # no --apply, no --provider: the whole table
```

## 8.3 — Read a full day of shadow

This is the cheap place to be wrong. Compare what the scheduler SAYS it would launch
against what the timers actually launched:

```
journalctl -u freehire-ingest-scheduler.service --since "-24h" -o cat | grep "would launch" \
  | awk '{print $4}' | sort | uniq -c | sort -rn | head -40
journalctl -u "freehire-ingest@*" --since "-24h" -o cat | grep "^Starting" \
  | sed 's/.*ingest //;s/\.\.\.//' | sort | uniq -c | sort -rn | head -40
```

The counts should track each provider's cadence. Investigate BEFORE cutting over:

- a provider in one list and not the other,
- a `REFUSED` line (a provider key no adapter answers to — the `habr_career` class),
- a `saturated` line appearing often (the cap is too low for the fleet's real shape).

## 8.4 — Cut over, in waves

**Read the HOST's enabled units as the source of truth for what to disable.** `deploy/` is
190 files adrift and must not drive this.

Start with a handful of cheap, hourly, unsharded providers and watch them for an hour:

```
for p in arbeitnow remoteok weworkremotely; do
  systemctl disable --now freehire-ingest@$p.timer
  schedule-board --provider=$p --manage --apply
done
```

Turn launches on once (this is the moment the scheduler stops being inert):

```
# add INGEST_SCHEDULER_APPLY=1 to /opt/freehire/.env, then
systemctl start freehire-ingest-scheduler.service
journalctl -u freehire-ingest-scheduler.service -n 30 --no-pager -o cat
systemctl list-units 'freehire-ingest-run-*' --all --no-pager
```

**Rollback for any provider is the reverse pair**, and it is always available:

```
schedule-board --provider=<p> --unmanage --apply
systemctl enable --now freehire-ingest@<p>.timer
```

Then widen in waves of ~20-40, checking `schedule-board` between waves for a provider whose
`LAST RUN` has stopped advancing. **The seven shard families go LAST** — they carry the
raised timeouts and the tightest pacing, so a mistake there costs the most.

## 8.5 — Drop the rollout gate. NOT OPTIONAL.

Once every provider is managed, `managed` must go: a rollout default that outlives its
rollout restores the exact failure this change removes (absence meaning unscheduled).
Add a migration dropping the column, and remove the `Managed` conjunct from
`Settings.Schedulable`, the `Unmanaged` bucket from `TickResult`, and `--manage/--unmanage`
from `cmd/schedule-board`.

## 9 — Retire the old path

Only after 8.5, and only after a full cycle of the slowest provider (paylocity: 24h) has
completed under the scheduler.

```
rm -f /etc/systemd/system/freehire-ingest@*.timer
rm -f /etc/systemd/system/freehire-ingest-*-shard@*
rm -f /etc/systemd/system/freehire-ingest@.service /etc/systemd/system/freehire-ingest-*-shard@.service
rm -f /opt/freehire/bin/gen-ingest-timers.sh /opt/freehire/bin/ingest-slot.sh
systemctl daemon-reload
```

Delete the same files from `deploy/systemd` and `deploy/bin` in git, then:

```
./deploy/check-drift.sh     # from a checkout; must exit 0
systemctl start freehire-autodeploy.timer
```

## 10.4 — The measurement that closes this

The one check that proves the fleet did not quietly lose a provider — which is the whole
reason this change exists. Every provider must have completed a run within its own cadence:

```
schedule-board | awk 'NR>1 && $6 != "disabled"'
```

Anything whose `LAST RUN` is older than its `CADENCE` is a provider that stopped crawling.
Under the old fleet that state was invisible; here it is one column.
