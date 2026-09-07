#!/usr/bin/env bash
# Cap how many ingest runs may execute at once. Installed at /opt/freehire/bin/,
# wrapped around every ingest ExecStart.
#
# Why: systemd fires ~137 independent per-file ingest timers staggered across the
# hour, but nothing bounds how many run together. Once a board's crawl outlasts its
# interval the runs stack up — measured 16 concurrent on a 16-vCPU box, driving
# `/proc/pressure/io` full to 19% and making every API query slow. CPUWeight/IOWeight
# on the unit only arbitrate against OTHER cgroups; they do not limit the fleet
# against itself.
#
# How: a counting semaphore built from INGEST_SLOTS flock'd files. Each run grabs a
# free slot and holds it for its lifetime (flock holds the lock for the child it
# execs). A run that cannot get a slot within INGEST_SLOT_WAIT skips this cycle
# rather than queueing into its own TimeoutStartSec and being killed mid-crawl —
# ingest is idempotent and hourly, so the next tick picks the board up. Skips are
# logged, never silent: a fleet that quietly stops crawling looks identical to a
# healthy one.
set -uo pipefail

# INGEST_SLOTS is the TOTAL concurrency the fleet may reach, and the heavy pool is carved
# OUT of it rather than added beside it. That matters because the figure lives in the
# host's /opt/freehire/.env, which overrides this default: a heavy pool ADDED to it would
# raise real concurrency by however many slots it holds, silently, on a host where the
# disk is the scarce resource and Postgres — not the crawl — is what pays for it.
SLOTS=${INGEST_SLOTS:-10}
# 4, measured. The pool's problem was never total work: a full sweep of the fleet costs
# ~11 slot-hours against 10 of capacity, so demand sits AT the ceiling rather than far
# past it. What starved the tail was RESIDENCY — four slots were permanently held by
# 25-65 minute crawls (greenhouse, apploi, careerplug, smartrecruiters, the workday
# shards), and the ~130 short runs an hour, which together cost 0.4 slot-hours, could not
# get in edgewise. Skipping them saved nothing, which is why 42% of cycles were skipped
# while average utilisation sat near half.
#
# Four is what was observed resident, so the split hands the long crawls the slots they
# already occupied and fences the rest off for the tail. Raising this takes slots FROM
# the tail, which is the thing being protected — measure the skip rate before moving it.
HEAVY_SLOTS=${INGEST_HEAVY_SLOTS:-4}
WAIT=${INGEST_SLOT_WAIT:-600}
DIR=${INGEST_SLOT_DIR:-/run/freehire/ingest-slots}
# The heavy roster is WRITTEN BY gen-ingest-timers.sh, which is where the list already
# lives (its HEAVY cohort plus the sharded providers). One owner, one copy: a second list
# maintained here would drift from the schedule silently, and a provider in one list but
# not the other is exactly the bug this split exists to avoid. A missing file is not an
# error — every run then takes the shared pool, which is the behaviour before this change.
ROSTER=${INGEST_HEAVY_ROSTER:-/opt/freehire/etc/ingest-heavy}
POLL=15
BUSY=75 # flock --conflict-exit-code: distinguishes "slot taken" from a real failure

[ $# -gt 0 ] || { echo "usage: ${0##*/} <command> [args...]" >&2; exit 64; }

# The provider is the wrapped command's first argument — `ingest <provider> [--shard=N/M]`
# — so a shard lands in the same pool as the provider it belongs to, which is the point:
# a workday shard is a 50-minute crawl whichever unit fires it.
provider=${2:-}
pool=shared
first=1
last=$((SLOTS - HEAVY_SLOTS))
if [ -n "$provider" ] && [ -r "$ROSTER" ] && grep -qxF -- "$provider" "$ROSTER" 2>/dev/null; then
	pool=heavy
	first=$((SLOTS - HEAVY_SLOTS + 1))
	last=$SLOTS
fi
# A misconfigured split must not leave a pool with no slots to take — that would turn
# every run of that pool into a skip, which reads exactly like a busy fleet.
if ((first > last)); then
	echo "ingest-slot: INGEST_HEAVY_SLOTS=$HEAVY_SLOTS leaves the $pool pool empty of $SLOTS; using all slots" >&2
	first=1
	last=$SLOTS
fi
mkdir -p "$DIR" 2>/dev/null || true

deadline=$((SECONDS + WAIT))
while :; do
	for ((i = first; i <= last; i++)); do
		flock -n -E "$BUSY" "$DIR/$i" "$@"
		rc=$?
		# Anything but BUSY means we held the slot and ran: propagate the real status.
		[ "$rc" -ne "$BUSY" ] && exit "$rc"
	done
	if ((SECONDS >= deadline)); then
		echo "ingest-slot: all $((last - first + 1)) $pool slots busy for ${WAIT}s, skipping this cycle: $*" >&2
		exit 0
	fi
	sleep "$POLL"
done
