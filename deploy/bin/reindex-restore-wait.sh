#!/usr/bin/env bash
# Waits for the detached from-pg semantic rebuild to finish, then restores the writers
# that were paused for a clean fill (ingest timers + shards, enrich, embed supervisor).
# Best-effort; runs regardless of the reindex exit status so services are never left
# paused. Launched via systemd-run so it survives an ssh disconnect.
LOG=/opt/freehire/reindex-restore.log
log(){ printf '%s %s\n' "$(date -u +%FT%TZ)" "$*" >>"$LOG"; }
log "waiter up; watching freehire-reindex-frompg"
while systemctl is-active --quiet freehire-reindex-frompg 2>/dev/null; do sleep 60; done
sleep 5
log "reindex-frompg finished; restoring writers"
# The enabled set is what timers.target.wants holds, and the names are systemd unit
# names — no whitespace to mishandle. The unquoted expansion below is the point: the
# list is passed to systemctl as separate arguments.
# shellcheck disable=SC2010
TIMERS=$(ls /etc/systemd/system/timers.target.wants/ 2>/dev/null | grep -E 'freehire-ingest.*\.timer')
# Each start is logged on its own so a partial restore names which unit failed;
# grouping the three into one redirect would lose that.
# shellcheck disable=SC2086,SC2129
systemctl start $TIMERS 2>>"$LOG"
systemctl start freehire-enrich.timer 2>>"$LOG"
systemctl start freehire-embed-supervisor.service 2>>"$LOG"
active=$(systemctl list-units 'freehire-ingest@*.timer' --no-legend 2>/dev/null | grep -c ' active ')
log "restored: ingest_timers_active=$active enrich=$(systemctl is-active freehire-enrich.timer) embed=$(systemctl is-active freehire-embed-supervisor.service)"
