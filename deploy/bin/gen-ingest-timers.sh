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

# An empty result is the only answer worth refusing on, and set -e already refuses a
# failed query. There is deliberately no "fewer than N providers looks wrong" floor: this
# script only ever creates and enables units — every systemctl disable below names one
# unit literally — so a short list generates fewer timers and retires nothing. A floor
# would guard nothing and would block a legitimately smaller catalog.
providers=$(psql "$DATABASE_URL" -tAc \
  "SELECT provider FROM boards WHERE status IN ('pending','active') GROUP BY provider ORDER BY provider")
if [ -z "$providers" ]; then
  echo "gen-ingest-timers: the catalog lists no live board — nothing to schedule" >&2
  exit 1
fi
mapfile -t PROVIDERS <<<"$providers"

# Boards measured (2026-07-31, 3h of journal) to average >=25 min per run — together
# 65% of all ingest busy-time, with oracle/paylocity/ukg/careerplug hitting
# TimeoutStartSec on EVERY run. Crawled hourly they never finished a sweep and kept
# ~11 runs resident at all times, saturating host I/O (pressure `full` 19%). An hourly
# timer on a 40-minute board only ever bought partial results. taleo was already on 3h
# for the same reason and joins them so it shares the spread below.
HEAVY="bamboohr icims paycom gupy mycareersfuture ukg careerplug jibe jazzhr vagas apple taleo"
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
        *)    cal="*:$(printf %02d "$min"):00" ;;
      esac ;;
  esac
  cat > "/etc/systemd/system/freehire-ingest@$n.timer" <<T
[Unit]
Description=timer ingest $n
[Timer]
OnCalendar=$cal
Persistent=true
RandomizedDelaySec=180
[Install]
WantedBy=timers.target
T
  systemctl enable --now "freehire-ingest@$n.timer" >/dev/null
  i=$((i+1))
done
echo "generated + enabled $i per-provider ingest timers"

# Retire the fingerprint-client 403-churners skipped above (they may carry a timer enabled
# before they were skipped), so they stop running until proxy support is wired for the
# fingerprint client. Mirrors the workday/eightfold legacy-timer cleanup below.
for n in bayt gulftalent; do
  systemctl disable --now "freehire-ingest@$n.timer" 2>/dev/null || true
done

# The 24 single-company providers that used to share custom.yml are ordinary catalog rows
# now, so the loop above already generated a timer for each. Retire the old bundled timer:
# it never named a real provider (every row inside custom.yml carried its OWN provider,
# never literally "custom"), so cmd/ingest custom finds nothing and exits 0.
systemctl disable --now freehire-ingest@custom.timer 2>/dev/null || true

# workday shards: one service template (--shard=N/6) + 6 timers, each every 6h at :40,
# offset one hour apart so a single ~1000-board shard runs per hour and finishes well
# within the 40-min timeout, together covering all ~6165 boards over a 6-hour cycle.
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
OnCalendar=*-*-* 0$((N-1))/6:40:00
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
OnCalendar=*-*-* 0$((N-1))/4:50:00
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
OnCalendar=*-*-* 0$((N-1))/4:15:00
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
OnCalendar=*-*-* 0$((N-1))/5:20:00
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
OnCalendar=*-*-* 0$((N-1))/4:42:00
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
