#!/usr/bin/env bash
# Web accept-queue watchdog for freehire (host-2).
#
# The SSR web process (Node/SvelteKit) has repeatedly wedged under concurrent
# ingest+meilisearch+postgres load: its LISTEN socket's accept queue fills up,
# the kernel silently drops new SYNs, and nginx reports "upstream timed out
# while connecting" for every real page — while /health (a lightweight
# endpoint served straight from the API) keeps answering 200, so a health-only
# check never sees it. Recurred three times (2026-07-31 full outage,
# 2026-08-05 partial, 2026-08-08 during a release) with the same signature.
# A wedged process clears in ~2s once restarted, but only once a human
# notices and runs it by hand — this replaces that hand with a timer.
#
# Checks the CURRENTLY ACTIVE color's web LISTEN socket: Recv-Q (the accept
# queue depth, not bytes — see ss(8)) against its own configured backlog
# (Send-Q, on a LISTEN socket). Recv-Q >= Send-Q means the queue is at
# capacity right now. Requires CONSECUTIVE_BAD consecutive bad checks before
# acting (one sample could be a benign burst) and a COOLDOWN between restarts
# (a genuinely overloaded box restarting every 15s would never finish warming
# up, and would turn a capacity problem into a crash-loop) — so this heals a
# wedge without masking a real capacity problem into invisibility.
#
# Installed at /opt/freehire/bin/web-watchdog.sh, driven by
# freehire-web-watchdog.timer (every 15s — the failure fills the queue within
# about a minute, so the check has to be tighter than site-alert's 2 min).
# Telegram delivery reuses the freehire bot: TELEGRAM_BOT_TOKEN +
# SITE_ALERT_CHAT_ID from /opt/freehire/.env (same channel as site-alert.sh —
# both are "something needed intervention" ops noise). Alerting is OPT-IN —
# if unset, the watchdog still restarts and logs, it just doesn't notify.
set -uo pipefail

S=/etc/nginx/snippets
ENV_FILE=/opt/freehire/.env
STATE_FILE=/run/freehire-web-watchdog.state
CONSECUTIVE_BAD=2 # ~30s of a full queue before acting
COOLDOWN_SECS=300 # don't restart more than once per 5 min

log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*"; }

active=$(readlink -f "$S/freehire-upstream-active.conf" 2>/dev/null || true)
case "$active" in
	*-blue.conf) color=blue port=8083 ;;
	*-green.conf) color=green port=8084 ;;
	*)
		log "cannot determine active color from '$active' — skipping"
		exit 0
		;;
esac

line=$(ss -ltn "sport = :${port}" 2>/dev/null | awk 'NR==2')
if [ -z "$line" ]; then
	log "no LISTEN socket on :${port} (color=${color}) — skipping"
	exit 0
fi
recvq=$(echo "$line" | awk '{print $2}')
sendq=$(echo "$line" | awk '{print $3}')
log "web@${color} :${port} Recv-Q=${recvq} Send-Q=${sendq}"

# count=<consecutive bad checks> last=<unix ts of last restart, 0 = never>
prev=$(cat "$STATE_FILE" 2>/dev/null || echo "count=0 last=0")
count=$(echo "$prev" | grep -oE 'count=[0-9]+' | cut -d= -f2)
last=$(echo "$prev" | grep -oE 'last=[0-9]+' | cut -d= -f2)
count=${count:-0}
last=${last:-0}
now=$(date +%s)

bad=0
if [ "${sendq:-0}" -gt 0 ] && [ "${recvq:-0}" -ge "${sendq:-0}" ]; then
	bad=1
fi

if [ "$bad" -eq 0 ]; then
	echo "count=0 last=${last}" >"$STATE_FILE"
	exit 0
fi

count=$((count + 1))
echo "count=${count} last=${last}" >"$STATE_FILE"

if [ "$count" -lt "$CONSECUTIVE_BAD" ]; then
	log "web@${color} accept queue full (${count}/${CONSECUTIVE_BAD} consecutive) — waiting for confirmation"
	exit 0
fi

if [ "$last" -gt 0 ] && [ $((now - last)) -lt "$COOLDOWN_SECS" ]; then
	log "web@${color} accept queue still full but restarted $((now - last))s ago (< ${COOLDOWN_SECS}s cooldown) — skipping"
	exit 0
fi

log "web@${color} accept queue full for ${count} consecutive checks — restarting freehire-web@${color}"
systemctl restart "freehire-web@${color}"
echo "count=0 last=${now}" >"$STATE_FILE"

token=$(grep -E '^TELEGRAM_BOT_TOKEN=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)
chat=$(grep -E '^SITE_ALERT_CHAT_ID=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)
text="🛠️ freehire $(hostname): web@${color} accept queue was full (Recv-Q=${recvq}/Send-Q=${sendq}) — auto-restarted freehire-web@${color}. Site may have been briefly unreachable before this ran; check load (ingest/meilisearch/postgres) if this repeats."
if [ -n "$token" ] && [ -n "$chat" ]; then
	if curl -sS -m 10 -o /dev/null \
		--data-urlencode "chat_id=${chat}" \
		--data-urlencode "text=${text}" \
		"https://api.telegram.org/bot${token}/sendMessage"; then
		log "alert sent to Telegram chat ${chat}"
	else
		log "alert send FAILED"
	fi
else
	log "ALERT (not sent, SITE_ALERT_CHAT_ID unset): ${text}"
fi

exit 0
