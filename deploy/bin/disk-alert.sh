#!/usr/bin/env bash
# Disk-space alert for freehire (host-2).
#
# Checks the root filesystem and, at or above THRESHOLD_PCT used, sends a Telegram
# message so a filling disk is caught BEFORE it takes down Postgres/Meili — a recurring
# outage (see the disk-full incident notes: an orphan `jobs_rebuild` Meili index has
# filled the disk to 100% several times). Read-only and best-effort: it never fails the
# way a broken backup would, so it can run every 15 min without noise.
#
# Installed at /opt/freehire/bin/disk-alert.sh, driven by freehire-disk-alert.timer.
# Telegram delivery reuses the freehire bot: TELEGRAM_BOT_TOKEN + DISK_ALERT_CHAT_ID from
# /opt/freehire/.env. Alerting is OPT-IN via DISK_ALERT_CHAT_ID — if it is unset the check
# still logs the usage but sends nothing, so installing the timer is harmless before the
# channel is configured.
set -uo pipefail

MOUNT=/
THRESHOLD_PCT=85
ENV_FILE=/opt/freehire/.env

log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*"; }

used=$(df --output=pcent "$MOUNT" | tail -1 | tr -dc '0-9')
avail=$(df -h --output=avail "$MOUNT" | tail -1 | tr -d ' ')
log "disk $MOUNT ${used}% used, ${avail} free (threshold ${THRESHOLD_PCT}%)"

if [ "${used:-0}" -lt "$THRESHOLD_PCT" ]; then
	exit 0
fi

# Above threshold — try to alert. Read only the two keys we need (never source the whole
# env, which would execute arbitrary content).
token=$(grep -E '^TELEGRAM_BOT_TOKEN=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)
chat=$(grep -E '^DISK_ALERT_CHAT_ID=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)
if [ -z "$token" ] || [ -z "$chat" ]; then
	log "ALERT: disk ${used}% used but Telegram alert not configured (set DISK_ALERT_CHAT_ID in $ENV_FILE)"
	exit 0
fi

text="⚠️ freehire $(hostname): disk ${MOUNT} at ${used}% used, ${avail} free. Free space before Postgres/Meili fail — check for an orphan jobs_rebuild Meili index."
if curl -sS -m 10 -o /dev/null \
	--data-urlencode "chat_id=${chat}" \
	--data-urlencode "text=${text}" \
	"https://api.telegram.org/bot${token}/sendMessage"; then
	log "alert sent to Telegram chat ${chat}"
else
	log "alert send FAILED"
fi
exit 0
