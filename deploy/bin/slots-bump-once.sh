#!/usr/bin/env bash
# One-off, approved 2026-09-05: raise INGEST_SLOTS 8 -> 10 in /opt/freehire/.env
# once the taxonomy backfill (backfill-derive) has finished, so the extra crawl
# concurrency does not stack on a box already at load ~25.
#
# The value 10 is the calibrated default in deploy/bin/ingest-slot.sh; the env
# file pinned the superseded 8, which is why the fleet skipped ~1111 cycles in
# 24h against 1008 real runs.
#
# No service restart needed: every ingest reads the env file at start.
set -uo pipefail

LOG=/opt/freehire/slots-bump.log
ENV=/opt/freehire/.env
BAK=/opt/freehire/.env.bak-slots-$(date -u +%Y%m%d-%H%M%S)

log() { echo "[$(date -u +%FT%TZ)] $*" >>"$LOG"; }

log "waiting for taxonomy-backfill-night to finish"

# Up to 12h, polled every 5 min. If it outlives that, do nothing and say so:
# an unattended bump into an unknown machine state is not what was approved.
for _ in $(seq 1 144); do
	s=$(systemctl is-active taxonomy-backfill-night.service)
	if [ "$s" != "active" ] && [ "$s" != "activating" ]; then
		log "backfill state=$s -> proceeding"
		break
	fi
	sleep 300
done

s=$(systemctl is-active taxonomy-backfill-night.service)
if [ "$s" = "active" ] || [ "$s" = "activating" ]; then
	log "GAVE UP: backfill still $s after 12h, INGEST_SLOTS left at 8"
	exit 1
fi

if ! grep -q '^INGEST_SLOTS=8$' "$ENV"; then
	log "SKIPPED: $ENV no longer holds INGEST_SLOTS=8 (now: $(grep '^INGEST_SLOTS=' "$ENV"))"
	exit 0
fi

cp -p "$ENV" "$BAK"
sed -i 's/^INGEST_SLOTS=8$/INGEST_SLOTS=10/' "$ENV"
log "applied: $(grep '^INGEST_SLOTS=' "$ENV") (backup $BAK)"
log "load: $(uptime)"
