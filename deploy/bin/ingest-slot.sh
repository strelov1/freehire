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

# 10, calibrated against the fleet rather than guessed. 8 was the first cut and ran
# short: ~4 slots sit occupied by 30-50 min crawls, leaving only 4 for the ~124 boards
# that finish in under a minute, so throughput settled near 104 runs/h against ~139/h of
# demand and the tail kept skipping. Note the heavy boards now run to completion where
# they used to be killed at TimeoutStartSec, so each costs more than the pre-change
# measurement suggested. Seam if the tail still skips: give heavy boards their own small
# pool so they cannot squat the shared one.
SLOTS=${INGEST_SLOTS:-10}
WAIT=${INGEST_SLOT_WAIT:-600}
DIR=${INGEST_SLOT_DIR:-/run/freehire/ingest-slots}
POLL=15
BUSY=75 # flock --conflict-exit-code: distinguishes "slot taken" from a real failure

[ $# -gt 0 ] || { echo "usage: ${0##*/} <command> [args...]" >&2; exit 64; }
mkdir -p "$DIR" 2>/dev/null || true

deadline=$((SECONDS + WAIT))
while :; do
	for ((i = 1; i <= SLOTS; i++)); do
		flock -n -E "$BUSY" "$DIR/$i" "$@"
		rc=$?
		# Anything but BUSY means we held the slot and ran: propagate the real status.
		[ "$rc" -ne "$BUSY" ] && exit "$rc"
	done
	if ((SECONDS >= deadline)); then
		echo "ingest-slot: all $SLOTS slots busy for ${WAIT}s, skipping this cycle: $*" >&2
		exit 0
	fi
	sleep "$POLL"
done
