#!/usr/bin/env bash
# Generate per-provider systemd timers for freehire ingest on host-2.
# One timer per provider in the boards catalog (except the ones sharded separately
# below), staggered across the hour; the heavy ones run every 3h. Mirrors the old
# prod crontab's per-provider isolation (a slow/hung provider can't block the others).
#
# The provider list comes from Postgres, not from a directory: the board catalog moved
# out of sources/*.yml into the boards table, and a provider is scheduled because it has
# live boards, not because a file with its name exists. A provider whose every board is
# retired stops being scheduled on the next run of this script; its timer is retired by
# the sweep at the end.
#
# This file is the record of what runs on the host. Nothing deploys it — copy it to
# /opt/freehire/bin/ after editing, like release.sh (see deploy/AGENTS.md).
set -euo pipefail
i=0

# The catalog is the schedule. DATABASE_URL comes from the host env file, the same one
# every worker unit loads.
# shellcheck disable=SC1091  # the host env file, not part of this repo
if [ -f /opt/freehire/.env ]; then set -a; . /opt/freehire/.env; set +a; fi

# An empty result is the only answer worth refusing on outright, and set -e already refuses
# a failed query.
#
# This comment used to argue that no "fewer than N providers looks wrong" floor was needed,
# because the script only ever created and enabled units. That stopped being true when the
# sweep at the very bottom gained the ability to retire a timer whose provider has left the
# catalogue — so the floor the old reasoning ruled out now guards exactly the case that
# reasoning relied on not existing. It lives with the sweep, not here, because a short list
# is only dangerous to the sweep: generation itself is still create-and-enable only.
# The LEFT JOIN is the cutover's one-owner rule, enforced here rather than trusted to an
# operator. A provider handed to cmd/ingest-scheduler (ingest_schedule.managed) must NOT
# also carry a static timer: the two ceilings cannot see each other, so a doubly-driven
# provider runs twice at once on a host calibrated for one. The runbook says to disable the
# timer and flip the flag as ONE step — but this script runs unattended at 04:40 and would
# have RECREATED the timer of every cut-over provider the same night, silently undoing the
# operator's half of it. LEFT, not INNER: a provider with no override row is unmanaged and
# still ours, which is the whole design of that table.
#
# The sweep at the bottom finishes the job: a provider that becomes managed drops out of
# this list, so its timer is retired on the next run without anyone naming it.
providers=$(psql "$DATABASE_URL" -tAc \
  "SELECT b.provider FROM boards b
     LEFT JOIN ingest_schedule s ON s.provider = b.provider
    WHERE b.status IN ('pending','active') AND COALESCE(s.managed, false) = false
    GROUP BY b.provider ORDER BY b.provider")
if [ -z "$providers" ]; then
  echo "gen-ingest-timers: the catalog lists no live board — nothing to schedule" >&2
  exit 1
fi
mapfile -t PROVIDERS <<<"$providers"

# The providers the scheduler owns. Read separately rather than derived from the list above:
# that list is what this script SHOULD generate, and knowing which names are missing from it
# BECAUSE they were cut over — rather than because the catalogue read failed — is exactly
# what the sweep's floor needs to tell apart. An empty result is normal and is not an error;
# before the cutover began there were none.
declare -A MANAGED=()
while read -r m; do
  [ -n "$m" ] && MANAGED[$m]=1
done < <(psql "$DATABASE_URL" -tAc \
  "SELECT provider FROM ingest_schedule WHERE managed ORDER BY provider")
# Every provider the per-provider loop below actually generated a timer for. The sweep at
# the bottom retires the enabled timers NOT in here, so it must be appended to at exactly
# one place: beside the `systemctl enable` that creates the timer, never beside the loop's
# `continue`s — a sharded provider is skipped there on purpose and its plain timer is meant
# to stay retired.
#
# A SET, not a list, so the sweep's membership test is a lookup rather than a scan of 240
# names per timer, and reads as the question it is asking.
declare -A GENERATED=()

# Boards measured (2026-07-31, 3h of journal) to average >=25 min per run — together
# 65% of all ingest busy-time, with oracle/paylocity/ukg/careerplug hitting
# TimeoutStartSec on EVERY run. Crawled hourly they never finished a sweep and kept
# ~11 runs resident at all times, saturating host I/O (pressure `full` 19%). An hourly
# timer on a 40-minute board only ever bought partial results. taleo was already on 3h
# for the same reason and joins them so it shares the spread below.
# ONE LINE, deliberately: membership is tested with `case " $HEAVY " in *" $n "*)`, which
# is a search for a space-delimited word. A newline inside the list would silently fail to
# match the entry before it and the entry after it, and nothing would report that.
#
# phenom..successfactors were added 2026-09-15 by the same metric as the rest:
# freehire_worker_last_run_duration_seconds, the binary's own runtime, which is the only
# figure that excludes the wait inside ingest-slot.sh. Measured: phenom 50.1, jobleads 50.0,
# wantapply 50.0, workable 50.0, trudvsem 48.3, freshteam 42.8, successfactors 39.5.
#
# The rule is RUNTIME, and the boundary is ~40min rather than 40: successfactors at 39.5
# sits below the line and is in anyway, because half a minute does not distinguish a crawl
# that fits an hourly timer from one that does not, and it was separately observed resident.
# Recorded rather than rounded away — a list whose stated rule excludes one of its own
# members teaches a reader to distrust the rule.
#
# The arithmetic is the whole story: a 50-minute crawl on an HOURLY timer holds 83% of one
# slot forever, so four of them is 3.3 of the six slots the shared pool had. Sampled every
# 20s for 8 minutes on 2026-09-15, workable, vk, trudvsem and freshteam held a shared slot
# in 48 of 48 samples and successfactors in 38 — six providers sitting on all six slots
# continuously, while the other ~230 split whatever was left. 837 of 1482 firings that day
# were skipped, and the ten providers whose timers had just been created were skipped on
# their FIRST cycle, every one.
#
# vk (25.0) and hrmos (34.0) were in that resident six and are deliberately NOT here. They
# are resident because they are frequent, not because one run is long, and the two need
# different answers: HEAVY buys a 3h cadence, which is the wrong instrument for a crawl
# that already fits its hour. Moving the >40min cohort out was measured to be enough — see
# the tail's 2h cadence below, which is what actually closed the gap. If either turns
# resident again once the pool has settled, it wants a cadence decision of its own, not
# membership here.
#
# At the 3h HEAVY cadence the same crawl holds 28% of a slot instead of 83%.
#
# What this does NOT fix: the fleet's total demand is ~26 slot-hours per cycle against 10
# slots, so hourly-for-everyone is unreachable however the pools are cut. That is what
# cmd/ingest-scheduler exists to solve -- it can order the work by how stale a provider
# actually is, which a wall-clock timer cannot.
HEAVY="bamboohr icims paycom gupy mycareersfuture ukg careerplug jibe jazzhr vagas apple taleo phenom jobleads wantapply workable trudvsem freshteam successfactors"

# SHARDED lists the providers generated as shard units further down instead of as one
# timer here. They are heavy by definition — sharding is what a provider gets when even
# the 3h HEAVY cadence could not finish it — so for the SLOT POOL they belong with HEAVY,
# even though for SCHEDULING they are skipped by the `continue`s in the loop below.
SHARDED="workday oracle paylocity eightfold join dayforce workstream adp adpmyjobs"

# Heavy for the POOL but not for the SCHEDULE. Measured 2026-09-07 from
# freehire_worker_last_run_duration_seconds — the binary's own runtime, which is the only
# figure that excludes the wait inside ingest-slot.sh: greenhouse 48min, apploi 46,
# smartrecruiters 38, lamoda 20, trakstar 19, adp 16, teamtailor 15. Each holds a slot for
# most of an hour, which is what starved the short tail; none was in HEAVY, so before this
# they squatted the shared pool the split exists to protect.
#
# Deliberately NOT moved into HEAVY: that would also drop them to a 3h cadence, and
# nothing measured here says their listings change slowly enough to justify that. The pool
# and the schedule are separate decisions, and this list is the one that says so.
#
# apploi was here and is gone (2026-09-15): it is no longer scheduled at all, so naming it
# as a pool tenant wrote a provider nothing can launch into /opt/freehire/etc/ingest-heavy.
# Its 46min above is left as recorded evidence of what the cohort looks like; it is history,
# not a live tenant.
HEAVY_POOL_ONLY="greenhouse smartrecruiters lamoda trakstar adp teamtailor"

# The roster ingest-slot.sh reads to decide which pool a run takes. Written here because
# the list already lives here: a second copy kept in the slot script would drift from the
# schedule silently, and a provider heavy in one file but not the other is exactly the
# failure the split exists to prevent. Written BEFORE the timers, so a run that fires
# mid-generation reads a complete roster rather than half of one.
ROSTER=/opt/freehire/etc/ingest-heavy
mkdir -p "$(dirname "$ROSTER")"
# shellcheck disable=SC2086  # the three lists are space-separated words on purpose;
# quoting them would print three lines, one per list, and the roster is one name per line.
printf '%s\n' $HEAVY $SHARDED $HEAVY_POOL_ONLY | sort -u > "$ROSTER.tmp"
mv "$ROSTER.tmp" "$ROSTER"

hi=0
for n in "${PROVIDERS[@]}"; do
  # workday (~6165 boards) 429-throttles too hard to finish in one 40-min run,
  # so it's crawled as 6 company-grouped shards (freehire-ingest-workday-shard@N),
  # every 6h, staggered one per hour — generated in the block below, not here.
  [ "$n" = workday ] && continue
  # oracle.yml (796 boards, per-posting detail fan-out) was still hitting TimeoutStartSec on
  # the 3h HEAVY cadence above — a single run only reached ~35% of the file before being
  # killed, so most boards were never revisited often enough to refresh last_seen_at or enter
  # the unseen sweep's crawled-company scope (issue #2017: 18% of oracle's live jobs stuck
  # unswept for days). Crawled as 4 board-sharded runs instead — generated below, not here.
  [ "$n" = oracle ] && continue
  # paylocity.yml (9477 boards — by far the largest file) was WAY past saveable on the 3h
  # HEAVY cadence: a run only reached ~288 boards (~3%) before TimeoutStartSec, and since
  # crawl order is fixed file order with no resume cursor, the same leading slice got hit
  # every cycle while the other 97% was never revisited at all (issue #2017: 74% of its
  # live jobs stuck unswept for days). Crawled as 24 board-sharded runs instead — generated
  # below, not here. 24 (not the eightfold/oracle-style 4) because even a 24-way split still
  # needs a raised TimeoutStartSec=4500 per shard to fit ~395 boards at this file's per-board
  # rate — see the paylocity shard block for the arithmetic.
  [ "$n" = paylocity ] && continue
  # adp tripled (2,798 -> 7,890 boards) on 2026-09-08 and inherited paylocity's exact failure:
  # a run reached ~2,300 boards before TimeoutStartSec and, with crawl order fixed and no
  # resume cursor, took the same leading slice every cycle — 4,414 boards had never been
  # attempted once. Crawled as 8 board-sharded runs instead, generated below.
  [ "$n" = adp ] && continue
  # adpmyjobs is ADP's other career-site product (added 2026-09-09). Fifteen times fewer
  # boards, but each carries ~161 open jobs against adp's 8.6, so it hit the same timeout from
  # the other direction — killed on 7 of its first 8 firings at ~52 of 498 boards. Also 8
  # shards, generated below.
  [ "$n" = adpmyjobs ] && continue
  # eightfold routes through the egress proxy (SOURCES_PROXY_URL) because its edge
  # IP-blocklists the prod IP. A few boards are enormous (nvidia/hp/citi: thousands of
  # jobs × per-job detail through 2 workers on one throttled proxy → ~20+ min each), so a
  # single hourly full-file run risks blowing the 40-min timeout. It's crawled as 4
  # board-sharded runs instead — generated in the eightfold block below, not here.
  [ "$n" = eightfold ] && continue
  # bayt/gulftalent egress via the Chrome-fingerprint client (NewFingerprintHTTP), which has
  # no proxy support, and both hard-403 the prod datacenter IP — so an hourly timer only
  # churns 403s and board_health noise without ingesting anything. Skip until proxy support is
  # wired for the fingerprint client; the disable loop after this loop retires any live timer.
  { [ "$n" = bayt ] || [ "$n" = gulftalent ]; } && continue
  # apploi's upstream API stopped honouring the `employer` parameter, and the adapter is
  # built entirely on it (api.apploi.com/v1/jobs?employer=<id>). Measured 2026-09-15 against
  # the live endpoint: `employer=39092`, `employer=999999999`, `employer=52601` and NO
  # employer parameter at all return byte-identical pages. Every one of the 5833 boards
  # therefore walks apploi's whole global catalogue instead of that employer's postings.
  #
  # Both halves of that are already in production data. The crawl cannot finish: it stops
  # at apploiMaxPages (100 pages, a hard Fetch failure by the fullBoardListing contract),
  # which is ~100 requests and ~5 minutes spent per board to store nothing -- 592 of 5833
  # boards carry that error, and a 50-minute run reaches 235 boards before systemd kills
  # it on TimeoutStartSec. And what it DID store before the endpoint changed is the same
  # posting once per board under a different employer each time: external_id
  # `41350:1498798|ontray` sits beside `53924:1498798|fulton-manor-care-center` and
  # `52204:1498798|magnet-aba-therapy` -- one real job, three companies, none of them
  # necessarily right.
  #
  # Skipped rather than fixed here because the fix is an ADAPTER rewrite, not a schedule:
  # the endpoint is a single global catalogue now, so apploi belongs as a BOARDLESS
  # provider crawled once, attributing each posting by its own `brand_name` field (the only
  # employer identity the payload still carries -- there is no employer id in it any more).
  # Until then an enabled timer holds a heavy slot for 50 minutes to ingest nothing.
  #
  # NOT resolved by this line: the ~1.47M open apploi rows already stored. Leaving them is
  # a deliberate hold, not an oversight -- closing them is a separate, reviewed decision.
  [ "$n" = apploi ] && continue
  # join.com meters by rate, not concurrency (internal/sources/pacer.go), and an hourly
  # full-file run at the paced rate can't clear ~4700 boards' worth of requests inside
  # TimeoutStartSec. Crawled as 5 board-sharded runs instead — generated below, not here.
  [ "$n" = join ] && continue
  # dayforce.yml: sharded 4 ways (TimeoutStartSec=4500), like oracle — hand-installed on
  # host2 (#66-ish, never reached this generator; found as drift while fixing the
  # provider-argument cutover, freehire#2357). Generated in the dayforce block below.
  [ "$n" = dayforce ] && continue
  # workstream.yml: sharded 2 ways, paced to ~0.5 req/s by its own origin — same
  # hand-installed-drift story as dayforce. Generated in the workstream block below.
  [ "$n" = workstream ] && continue
  min=$(( (i*41) % 60 ))
  # Reset per provider: this is one long loop in one shell, so a value set in a provider's
  # case arm below would otherwise carry into every provider generated after it — and the
  # only arm that sets it (adzuna) turns catch-up OFF, which is the direction that fails
  # silently. Everything else wants Persistent=true.
  persistent=true
  # Most boards crawl hourly, staggered across the minutes of the hour. reed has a
  # per-hour API request quota its full crawl blows past (403 "exceeded your per-hour
  # request limit"), so it crawls every 6h to stay under it.
  case " $HEAVY " in
    *" $n "*)
      # A heavy board moves to 3h, and gets spread across the WHOLE 3h cycle rather
      # than only across the minutes of one hour: offsetting the starting hour too
      # keeps 15 boards that each run 25-40 min from all landing on 00:00 / 03:00 /
      # 06:00 together. 15 boards over 3 hour-offsets x 60 minutes; same trick the
      # workday/eightfold shards use below. Spreading is best-effort — run lengths
      # vary — so ingest-slot.sh still enforces the hard ceiling.
      hh=$(( hi % 3 )); hm=$(( (hi*17) % 60 )); hi=$((hi+1))
      cal="*-*-* $(printf %02d "$hh")/3:$(printf %02d "$hm"):00" ;;
    *)
      case "$n" in
        reed) cal="*-*-* 00/6:$(printf %02d "$min"):00" ;;
        # adzuna is reed's case with a second twist. Its free API states 25/min, 250/day,
        # 1000/week and 2500/month, and the crawl's whole budget is
        # boards (4) x adzunaMaxPages (15) x runs/day, so the cadence is the only leg
        # of it that lives out here: four runs is 240/day, one more would clear 250.
        # Hourly (the default branch) was 4x that and bought nothing — until the adapter
        # sent sort_by=date, Adzuna answered in relevance order, stable between runs, so
        # 23 of 24 daily firings re-read the same slice.
        #
        # Persistent=false, unlike every other timer here: a catch-up run after a reboot
        # or a daemon-reload is a FIFTH run that day, which is 300 requests and over the
        # daily ceiling. Everywhere else a missed crawl is worth catching up; here the
        # crawl reads a recency slice, so a late run mostly re-reads what the next one
        # would have, and the ceiling is worth more than the catch-up.
        adzuna) cal="*-*-* 00/6:22:00"; persistent=false ;;
        # The tail: every 2h, spread across BOTH hours of the cycle as well as across
        # the minutes, the same two-axis trick the HEAVY branch above uses.
        #
        # It was hourly, and hourly does not fit. Measured 2026-09-15: ~230 tail
        # providers at ~150s of real crawl each is ~9.6 slot-hours per sweep, against
        # the 5 slots the shared pool has once HEAVY_SLOTS took its 5. Asking for that
        # every hour is 190% of capacity, so ingest-slot.sh threw away what would not
        # fit -- 837 of 1482 firings skipped in 24h, 52-58% in every window measured
        # since, unchanged by moving the resident crawls to HEAVY (freehire#2859),
        # because re-pooling cannot create capacity that is not there.
        #
        # At 2h the same demand is ~4.8 slot-hours per hour against 5, which fits.
        #
        # This LOOKS like halving freshness and is the opposite. An hourly timer that
        # is skipped 59% of the time already crawls a provider every ~2.4h on average,
        # and which hour it lands in is a lottery -- wellfound and recruiterflow lost
        # it for over a day straight, with no failure to show for it. A 2h timer that
        # actually runs is both fresher on average and, unlike the hourly one, a figure
        # anyone can reason about. Persistent=true still catches up a missed cycle.
        #
        # The real fix is cmd/ingest-scheduler, which can order the work by how stale a
        # provider is instead of by a wall clock. This is what the wall clock can do.
        *)    cal="*-*-* $(printf %02d "$(( i % 2 ))")/2:$(printf %02d "$min"):00" ;;
      esac ;;
  esac
  cat > "/etc/systemd/system/freehire-ingest@$n.timer" <<T
[Unit]
Description=timer ingest $n
[Timer]
OnCalendar=$cal
Persistent=$persistent
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
T
  systemctl enable --now "freehire-ingest@$n.timer" >/dev/null
  GENERATED[$n]=1
  i=$((i+1))
done
echo "generated + enabled $i per-provider ingest timers"

# Retire the fingerprint-client 403-churners skipped above (they may carry a timer enabled
# before they were skipped), so they stop running until proxy support is wired for the
# fingerprint client. Mirrors the workday/eightfold legacy-timer cleanup below.
# apploi joins them rather than relying on the sweep below. The sweep would catch it, but
# the sweep is allowed to refuse (the 80% floor), and a refusal would leave apploi holding
# a heavy slot for 50 minutes per run to ingest nothing — exactly the cost the skip exists
# to stop. A provider skipped on purpose is retired on purpose.
for n in bayt gulftalent apploi; do
  systemctl disable --now "freehire-ingest@$n.timer" 2>/dev/null || true
done

# The 24 single-company providers that used to share custom.yml are ordinary catalog rows
# now, so the loop above already generated a timer for each. Retire the old bundled timer:
# it never named a real provider (every row inside custom.yml carried its OWN provider,
# never literally "custom"), so cmd/ingest custom finds nothing and exits 0.
systemctl disable --now freehire-ingest@custom.timer 2>/dev/null || true

# workday shards: one service template (--shard=N/6) + 6 timers, each every 12h at :40,
# offset TWO hours apart so a single ~1000-board shard runs every other hour and finishes
# well within the 40-min timeout, together covering all ~6165 boards over a 12-hour cycle.
#
# It was every 6h (one shard per hour) until 2026-09-15, and that is where the heavy pool's
# capacity was going. Measured that day: the six sharded providers firing one shard per hour
# each demanded 5.83 slots against a heavy pool of 5, before any of the 19 HEAVY providers
# or the 6 pool-only ones were counted. The shards themselves skipped 34 of 66 firings in
# 24h, and bamboohr -- 12,004 boards, 9,407 technical postings, 11,204 companies -- went 32
# hours without a crawl because every one of its 3h firings found five busy slots.
#
# Halving the shard rate costs almost nothing REAL: at a 51% skip rate these providers were
# already sweeping about every 12h, just unpredictably and while burning a slot's worth of
# 600-second waits to find out. The new cadence states what was already happening and hands
# the waiting back to the pool. The offsets are respread to (N-1)*2 rather than left at
# N-1: doubling the period without moving them would clump all six shards into hours 0-5
# and 12-17 and leave half the day empty.
# ExecStart uses hire-current (the active blue/green release), matching the workers.
cat > /etc/systemd/system/freehire-ingest-workday-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest workday shard %i/6
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=3000
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest workday --shard=%i/6
UNIT
# Retire any legacy hourly full-file workday timer so it can't race the shards.
systemctl disable --now freehire-ingest@workday.timer 2>/dev/null || true
for N in 1 2 3 4 5 6; do
  cat > "/etc/systemd/system/freehire-ingest-workday-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest workday shard $N/6
[Timer]
OnCalendar=*-*-* $(printf %02d $(( (N-1)*2 )))/12:40:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-workday-shard@$N.timer" >/dev/null
done
echo "generated + enabled 6 workday shard timers"

# eightfold shards: one service template (--shard=N/4) + 4 timers, each every 4h, offset
# one hour apart so a single ~13-board shard runs per hour. Sharding isolates the giant
# boards (nvidia/hp/citi) so one slow board can't starve the rest or blow the timeout;
# staggering keeps the shards from contending for the single egress proxy IP. The proxy
# itself is env-driven (SOURCES_PROXY_URL in /opt/freehire/.env, read via EnvironmentFile).
cat > /etc/systemd/system/freehire-ingest-eightfold-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest eightfold shard %i/4
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=3000
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest eightfold --shard=%i/4
UNIT
# Retire any legacy hourly full-file eightfold timer so it can't race the shards.
systemctl disable --now freehire-ingest@eightfold.timer 2>/dev/null || true
for N in 1 2 3 4; do
  cat > "/etc/systemd/system/freehire-ingest-eightfold-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest eightfold shard $N/4
[Timer]
OnCalendar=*-*-* $(printf %02d $(( (N-1)*2 )))/8:50:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-eightfold-shard@$N.timer" >/dev/null
done
echo "generated + enabled 4 eightfold shard timers"

# oracle shards: one service template (--shard=N/4) + 4 timers, each every 4h, offset one
# hour apart so a single ~199-board shard runs per hour (measured ~6.82s/board including its
# per-posting detail fan-out, so a shard finishes in ~23min, comfortably inside the 3000s
# timeout) and the whole 796-board file cycles once every 4h — well inside the 48h unseen-sweep
# grace window. See issue #2017: the un-sharded hourly-then-3h timer never finished a pass.
cat > /etc/systemd/system/freehire-ingest-oracle-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest oracle shard %i/4
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=3000
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest oracle --shard=%i/4
UNIT
# Retire any legacy oracle timer (hourly, then 3h HEAVY) so it can't race the shards.
systemctl disable --now freehire-ingest@oracle.timer 2>/dev/null || true
for N in 1 2 3 4; do
  cat > "/etc/systemd/system/freehire-ingest-oracle-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest oracle shard $N/4
[Timer]
OnCalendar=*-*-* $(printf %02d $(( (N-1)*2 )))/8:15:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-oracle-shard@$N.timer" >/dev/null
done
echo "generated + enabled 4 oracle shard timers"

# paylocity shards: one service template (--shard=N/24) + 24 timers, each once a day at a
# distinct hour, so a single ~395-board shard runs per hour and the whole 9477-board file
# cycles once every 24h — comfortably inside the 48h unseen-sweep grace window, with the
# fixed round-robin (not contiguous-range) shard assignment in internal/sources/shard.go
# giving every shard an even mix regardless of the file's GUID-random board order. Measured
# ~10.42s/board on the un-sharded run (288 boards / 3000s), so a shard's own budget needs
# raising past the generic 3000s template: 395 boards * 10.42s =~ 4117s, so
# TimeoutStartSec=4500 here (not the 3000s every other provider uses) leaves ~6min margin
# rather than shaving the shard count down to fit the generic timeout. See issue #2017.
cat > /etc/systemd/system/freehire-ingest-paylocity-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest paylocity shard %i/24
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=4500
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest paylocity --shard=%i/24
UNIT
# Retire any legacy paylocity timer (hourly, then 3h HEAVY) so it can't race the shards.
systemctl disable --now freehire-ingest@paylocity.timer 2>/dev/null || true
for N in $(seq 1 24); do
  cat > "/etc/systemd/system/freehire-ingest-paylocity-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest paylocity shard $N/24
[Timer]
OnCalendar=*-*-* $(printf %02d $((N-1))):25:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-paylocity-shard@$N.timer" >/dev/null
done
echo "generated + enabled 24 paylocity shard timers"

# adp shards: one service template (--shard=N/8) + 8 timers, one every 3h, so the whole
# catalogue cycles once every 24h.
#
# adp grew from 2,798 boards to 7,890 on 2026-09-08 and immediately landed in the state
# paylocity was in above: crawl order is fixed with no resume cursor, so the same leading
# slice is taken every cycle. Measured 2026-09-09 across three runs — progress 2182, 2248
# and 2311 of 7,890 before TimeoutStartSec killed each one, and 4,414 boards (56%) with no
# board_health row at all, meaning they had never been attempted once. Six of nine real runs
# in 24h ended 'timeout'.
#
# 8 shards, not paylocity's 24: an adp board is small (26,131 open jobs over 3,033 boards
# with any = 8.6 each), so a board is one listing call plus its detail fan-out — measured
# ~3s under the provider's 5 req/s pacer. 7,890/8 = 986 boards a shard =~ 2,958s, inside the
# raised TimeoutStartSec=4500 with margin rather than shaved to fit the generic 3000s.
cat > /etc/systemd/system/freehire-ingest-adp-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest adp shard %i/8
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=4500
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest adp --shard=%i/8
UNIT
# Retire the legacy hourly timer so it can't race the shards.
systemctl disable --now freehire-ingest@adp.timer 2>/dev/null || true
for N in $(seq 1 8); do
  cat > "/etc/systemd/system/freehire-ingest-adp-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest adp shard $N/8
[Timer]
OnCalendar=*-*-* $(printf %02d $(( (N-1) * 3 ))):35:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-adp-shard@$N.timer" >/dev/null
done
echo "generated + enabled 8 adp shard timers"

# adpmyjobs shards: the same 8-way split for ADP's other career-site product, added
# 2026-09-09 with 498 boards and killed on 7 of its 8 firings in the first day, reaching
# ~52 boards each time.
#
# The same shard count for a fifteenth of the boards, because the boards are the opposite
# shape: 26,890 open jobs over 167 crawled boards = 161 each, against adp's 8.6. A MyJobs
# board is therefore ~58s of paced detail fan-out (measured: 52 boards in one 3000s run),
# so 498/8 = 62 boards a shard =~ 3,596s — the same 4500s budget, reached from the other
# direction. Sharding is what fixes the timeout; the pacer is a separate lever, and it is
# deliberately not touched here: board_health carries zero failures for this provider, which
# says the current rate is safe, not that a higher one would be.
cat > /etc/systemd/system/freehire-ingest-adpmyjobs-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest adpmyjobs shard %i/8
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=4500
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest adpmyjobs --shard=%i/8
UNIT
# Retire the legacy hourly timer so it can't race the shards.
systemctl disable --now freehire-ingest@adpmyjobs.timer 2>/dev/null || true
for N in $(seq 1 8); do
  cat > "/etc/systemd/system/freehire-ingest-adpmyjobs-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest adpmyjobs shard $N/8
[Timer]
OnCalendar=*-*-* $(printf %02d $(( (N-1) * 3 ))):45:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-adpmyjobs-shard@$N.timer" >/dev/null
done
echo "generated + enabled 8 adpmyjobs shard timers"

# join shards: one service template (--shard=N/5) + 5 timers, each every 5h at :20, offset one
# hour apart so a single shard runs per hour and the whole ~4749-board file cycles once every
# 5h. join.com meters by REQUEST RATE, not concurrency (internal/sources/pacer.go) — the pace
# lives in that paced client, not here, so this template carries no extra throttling of its
# own. Raised from 4 to 5 shards alongside a pace drop to 1.5 req/s (issue #2094: prod showed
# refusals accelerating through a long run at 2 req/s, which reads as a cumulative budget on
# top of the per-second one); at 1.5 req/s a shard takes ~35min of requests, which only 5-way
# splitting keeps under the 3000s timeout with headroom (TestJoinPaceFitsTheRunBudget pins the
# arithmetic on the Go side). These 5 timers were deployed straight to host2 before this
# generator carried them — this block brings the script back in sync with what is actually
# running, so a future re-run of this script does not resurrect the retired
# freehire-ingest@join.timer underneath them.
cat > /etc/systemd/system/freehire-ingest-join-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest join shard %i/5
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=3000
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest join --shard=%i/5
UNIT
# Retire any legacy hourly full-file join timer so it can't race the shards.
systemctl disable --now freehire-ingest@join.timer 2>/dev/null || true
for N in 1 2 3 4 5; do
  cat > "/etc/systemd/system/freehire-ingest-join-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest join shard $N/5
[Timer]
OnCalendar=*-*-* $(printf %02d $(( (N-1)*2 )))/10:20:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-join-shard@$N.timer" >/dev/null
done
echo "generated + enabled 5 join shard timers"

# dayforce shards: one service template (--shard=N/4) + 4 timers, each every 4h at :42,
# offset one hour apart. Hand-installed on host2 outside this generator (found as drift
# while fixing the provider-argument cutover, freehire#2357) — folded in here so a future
# regen reproduces it instead of dropping it. TimeoutStartSec=4500, like oracle/paylocity's
# per-posting-detail-fan-out boards.
cat > /etc/systemd/system/freehire-ingest-dayforce-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest dayforce shard %i/4
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
TimeoutStartSec=4500
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest dayforce --shard=%i/4
UNIT
systemctl disable --now freehire-ingest@dayforce.timer 2>/dev/null || true
for N in 1 2 3 4; do
  cat > "/etc/systemd/system/freehire-ingest-dayforce-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest dayforce shard $N/4
[Timer]
OnCalendar=*-*-* $(printf %02d $(( (N-1)*2 )))/8:42:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-dayforce-shard@$N.timer" >/dev/null
done
echo "generated + enabled 4 dayforce shard timers"

# workstream shards: one service template (--shard=N/2) + 2 timers, every 6h at :57,
# offset 3 hours apart. workstream paces to ~0.5 req/s by its own origin's IP metering —
# a shard is ~40min steady-state, longer on the first hydrating pass (see the comment
# carried into the service unit below). Same hand-installed-drift story as dayforce.
cat > /etc/systemd/system/freehire-ingest-workstream-shard@.service <<'UNIT'
[Unit]
Description=freehire ingest workstream shard %i/2
After=network.target postgresql.service meilisearch.service
[Service]
Type=oneshot
User=freehire
WorkingDirectory=/opt/freehire/src/hire-current
EnvironmentFile=/opt/freehire/.env
CPUWeight=40
IOWeight=40
# Sharded, and longer than the plain ingest unit's 3000s, because workstream is paced to
# ~0.5 req/s — its origin meters hard by IP. A shard is ~118 boards: in steady state that is
# ~1,200 requests (the listing walk plus re-hydrating the third of postings the non-tech filter
# rejects, which are never stored and so never `seen`), around 40 minutes. The FIRST crawl
# hydrates every posting instead — ~4,700 requests, over two hours — and will hit this timeout
# a couple of times. That is expected and safe: a run that times out still persists what it
# hydrated and the sweep only closes companies it actually crawled, so the backlog shrinks
# across runs rather than restarting.
TimeoutStartSec=4500
ExecStart=/opt/freehire/bin/ingest-slot.sh /opt/freehire/src/hire-current/ingest workstream --shard=%i/2
UNIT
systemctl disable --now freehire-ingest@workstream.timer 2>/dev/null || true
for N in 1 2; do
  cat > "/etc/systemd/system/freehire-ingest-workstream-shard@$N.timer" <<TIMER
[Unit]
Description=timer ingest workstream shard $N/2
[Timer]
OnCalendar=*-*-* 0$(( (N-1)*3 ))/6:57:00
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
TIMER
  systemctl enable --now "freehire-ingest-workstream-shard@$N.timer" >/dev/null
done
echo "generated + enabled 2 workstream shard timers"

# The sweep. Retires the per-provider timer of a provider that has LEFT the catalogue —
# every board of it retired, rejected, or deleted.
#
# The header of this file claimed for a long time that this already happened ("its timer is
# retired by the sweep at the end"). It did not: every `systemctl disable` above names one
# unit literally, so a provider that dropped out kept firing forever. Found 2026-09-15,
# when three boards whose provider no adapter answers to (globalpayments, justjoin,
# wantedkr — rows that predate boardcatalog's insert-time registry check) were marked
# rejected and their timers went on running anyway. Prose about code is tested by nothing.
#
# The floor is what makes this safe, and it is not decoration: the moment a run can RETIRE
# a timer, a query that returns a short list stops being harmless and starts being a
# fleet-wide outage that looks like a successful run. 80% is deliberately loose — a real
# catalogue does not shed a fifth of its providers between two daily runs, and a wave of
# board retirements that legitimately does is worth a human looking at it once.
# Collected in ONE pass, because `systemctl is-enabled` forks a process and the floor and
# the sweep both need the same answer: asking twice for each of ~254 timers took this run
# from 14 seconds to over ten minutes.
#
# Already-disabled units never enter the list, so the sweep leaves them alone rather than
# disabling them again — the literal disables above own those, and redoing their work here
# would hide which list a retirement came from.
enabled=()
floor_base=0
for f in /etc/systemd/system/freehire-ingest@*.timer; do
  [ -e "$f" ] || continue
  u=${f##*/}
  [ "$(systemctl is-enabled "$u" 2>/dev/null)" = enabled ] || continue
  p=${u#freehire-ingest@}
  p=${p%.timer}
  enabled+=("$p")
  # A provider handed to the scheduler is SWEPT (it is still in `enabled`) but does not
  # count toward the floor. Retiring its timer is the intended outcome of the cutover, not
  # evidence that the catalogue read came back short — and counting it made the guard fire
  # on exactly the operation it was supposed to permit. Measured 2026-09-16: a wave of 60
  # providers took generation to 169 against 229 enabled, 73.8%, and the sweep refused,
  # leaving all 60 driven by BOTH the scheduler and their static timer — the one state the
  # cutover forbids. Excluding them compares what the run generated against what it was
  # SUPPOSED to generate, which is the question the floor was always asking.
  [ -n "${MANAGED[$p]:-}" ] || floor_base=$((floor_base+1))
done

swept=0
refused=0
if [ "${#GENERATED[@]}" -lt $(( floor_base * 8 / 10 )) ]; then
  # Counted, not just logged. A refusal means the catalogue query answered with far less
  # than the fleet — which is the shape this guard exists to catch, and a run that only
  # writes it to stderr and exits 0 IS that shape: successful-looking and wrong. The
  # heartbeat below publishes this, so a refusal is visible without reading journald.
  refused=1
  echo "gen-ingest-timers: generated ${#GENERATED[@]} timers against $floor_base enabled and unmanaged — refusing to sweep" >&2
else
  for p in "${enabled[@]}"; do
    [ -n "${GENERATED[$p]:-}" ] && continue
    # Reported by what systemctl DID, not by what was attempted: the `|| true` keeps one
    # stuck unit from ending the run, and counting a failed disable as a retirement would
    # make the summary claim a fleet state that is not there.
    if ! systemctl disable --now "freehire-ingest@$p.timer" >/dev/null 2>&1; then
      echo "gen-ingest-timers: could not disable freehire-ingest@$p.timer — still enabled" >&2
      continue
    fi
    # States what this run OBSERVED, not why. A provider reaches here for two different
    # reasons — its boards left the catalogue, or a `continue` above skipped it on purpose
    # (apploi, bayt, the sharded ones) — and a message that asserts the first sends a
    # reader hunting for boards that are still there. Same reason the summary below counts
    # rather than explains.
    echo "gen-ingest-timers: retired $p — this run generated no timer for it"
    swept=$((swept+1))
  done
  # An `if`, not `[ ... ] && echo`. The reason first given here was that the && form exits
  # under `set -e` when there is nothing to sweep; that is FALSE and was never tested --
  # the shell exempts a command that is not the last of an AND-OR list, so it survives
  # mid-script and only leaks a non-zero status when it is the final statement. The `if`
  # earns its place for the smaller reason: it cannot acquire that fault by being moved.
  if [ "$swept" -gt 0 ]; then
    echo "retired $swept timer(s) this run did not generate"
  fi
fi

# A heartbeat, published the way every other periodic worker here publishes one. What this
# watches is not whether a crawl succeeded -- board_health answers that -- but whether the
# SCHEDULE is still being derived from the catalog at all. On 2026-09-15 it was not: the
# script had last run six days earlier, and 12 providers with live boards (eures with 31 of
# them, wellfound with 11) had no timer and were never crawled, while every unit on the host
# stayed green because a provider nothing runs cannot fail.
#
# Deliberately NOT a metric for the skip rate: `ingest-slot.sh` already publishes
# freehire_ingest_slot_skips_total, and the healthy value there is high (837 of 1482 firings
# skipped in the 24h to 2026-09-15 -- the fleet is oversubscribed by design and the
# semaphore is what keeps it from taking the host down). A threshold on that would fire
# every day and teach the reader to ignore it. Coverage has a healthy value of zero; the
# skip rate does not.
if [ -n "${PROM_TEXTFILE_DIR:-}" ] && [ -d "$PROM_TEXTFILE_DIR" ]; then
  out=$PROM_TEXTFILE_DIR/gen-ingest-timers.prom
  {
    echo "# HELP freehire_ingest_timers_generated Per-provider ingest timers written from the boards catalog."
    echo "# TYPE freehire_ingest_timers_generated gauge"
    echo "freehire_ingest_timers_generated $i"
    echo "# HELP freehire_ingest_timers_last_run_seconds Unix time the ingest timer generator last completed."
    echo "# TYPE freehire_ingest_timers_last_run_seconds gauge"
    echo "freehire_ingest_timers_last_run_seconds $(date +%s)"
    # 1 means the run declined to sweep because it generated far fewer timers than the
    # fleet currently has enabled. The heartbeat above stays fresh either way -- a refused
    # sweep is a COMPLETED run -- so without this the one outcome the floor exists to make
    # visible would be the one outcome nothing can see.
    echo "# HELP freehire_ingest_timers_sweep_refused 1 when the run declined to retire timers because its catalogue read looked short."
    echo "# TYPE freehire_ingest_timers_sweep_refused gauge"
    echo "freehire_ingest_timers_sweep_refused $refused"
    echo "# HELP freehire_ingest_timers_retired Timers this run retired because it generated none for that provider."
    echo "# TYPE freehire_ingest_timers_retired gauge"
    echo "freehire_ingest_timers_retired $swept"
  } > "$out.tmp" && mv "$out.tmp" "$out"
fi

# Every timer file above is REWRITTEN on each run, and `systemctl enable` on a unit that is
# already enabled links nothing new, so systemd goes on running the schedule it parsed the
# last time it reloaded. An edited OnCalendar therefore reaches the fleet only here. This
# mattered little while the script was run by hand and the operator reloaded out of habit;
# it matters now that freehire-gen-ingest-timers.timer runs it unattended.
systemctl daemon-reload
