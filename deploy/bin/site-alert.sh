#!/usr/bin/env bash
# Site + memory + load alert for freehire (host-2).
#
# Every run checks four things and, on any failing, sends a Telegram message:
#   1. https://freehire.me/health returns HTTP 200 (site up, DB reachable — see
#      internal/handler/health.go, which itself pings Postgres).
#   2. A real API endpoint answers 200 within API_TIMEOUT seconds — the check
#      /health is too cheap to be (see the api check below).
#   3. Available RAM is at or above MIN_FREE_PCT of total (catches an OOM
#      before the kernel starts killing processes).
#   4. 1-minute load average is under MAX_LOAD_PER_CORE x nproc (catches CPU
#      starvation, which /health does not see — see the load check below).
#
# Installed at /opt/freehire/bin/site-alert.sh, driven by freehire-site-alert.timer
# (every 2 min — faster than the 15-min disk-alert because a site outage is
# user-visible immediately). Telegram delivery reuses the freehire bot:
# TELEGRAM_BOT_TOKEN + SITE_ALERT_CHAT_ID from /opt/freehire/.env. Alerting is
# OPT-IN via SITE_ALERT_CHAT_ID — if unset the check still logs, so installing
# the timer is harmless before the channel is configured.
#
# State-debounced: at a 2-min cadence, alerting on every run while a problem
# persists would be noise. A state file remembers the last status per check
# and only sends on a transition (bad→worse, or back to ok) plus one reminder
# every REMINDER_EVERY runs while still bad.
set -uo pipefail

URL=https://freehire.me/health
# The endpoint the site is useless without, requested the way a visitor's browser
# requests it: through Cloudflare, so an edge failure counts as an outage here.
# limit=1 keeps it cheap enough to run every 2 minutes.
API_URL='https://freehire.me/api/v1/jobs?limit=1'
API_TIMEOUT=10
MIN_FREE_PCT=10
# Tenths of a core, per core: 20 means "alert once load1 reaches 2x nproc".
# The 2026-08-16 outage sat at 28-30 on 16 cores, so 1.5x (24) looked like the
# obvious threshold — until measurement: a normal night of ingest fan-out plus
# a running embed backfill already carries 23-24. At 1.5x this alert would page
# for healthy work, and an alert that cries wolf is worse than none.
#
# Raising it costs less coverage than it looks. Load is the coarse signal here;
# the api check above is the precise one, because it measures the damage rather
# than a cause — during that outage it would have tripped on the very first run
# (61s against a 10s deadline) while load was still climbing.
MAX_LOAD_PER_CORE_X10=20
ENV_FILE=/opt/freehire/.env
STATE_FILE=/run/freehire-site-alert.state
REMINDER_EVERY=10 # ~20 min at a 2-min cadence

log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*"; }

token=$(grep -E '^TELEGRAM_BOT_TOKEN=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)
chat=$(grep -E '^SITE_ALERT_CHAT_ID=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-)

send() {
	local text=$1
	if [ -z "$token" ] || [ -z "$chat" ]; then
		log "ALERT (not sent, SITE_ALERT_CHAT_ID unset): $text"
		return
	fi
	if curl -sS -m 10 -o /dev/null \
		--data-urlencode "chat_id=${chat}" \
		--data-urlencode "text=${text}" \
		"https://api.telegram.org/bot${token}/sendMessage"; then
		log "alert sent to Telegram chat ${chat}: $text"
	else
		log "alert send FAILED: $text"
	fi
}

# key=count, one per check, e.g. "site=3 api=0 mem=0 load=0". A key absent from
# an older state file starts at 0 via check()'s default, so adding a check needs
# no migration and no wiping of /run.
read -r -a prev_state < <(cat "$STATE_FILE" 2>/dev/null || echo "site=0 api=0 mem=0 load=0")
declare -A count
for kv in "${prev_state[@]}"; do count[${kv%%=*}]=${kv##*=}; done

# min_n delays the first alert until the check has failed that many runs in a
# row. Defaults to 1 (alert immediately); the load check raises it, because a
# reindex or an ingest fan-out spikes load for a minute and recovers on its own.
# The recovery notice is gated on the same threshold, so a transient blip that
# never alerted never announces itself as fixed either.
check() {
	local name=$1 bad=$2 bad_text=$3 ok_text=$4 min_n=${5:-1}
	local n=${count[$name]:-0}
	if [ "$bad" = "1" ]; then
		n=$((n + 1))
		if [ "$n" -eq "$min_n" ] || { [ "$n" -gt "$min_n" ] && [ $((n % REMINDER_EVERY)) -eq 0 ]; }; then
			send "🔴 freehire $(hostname): ${bad_text} (failing ${n}x)"
		fi
	else
		[ "$n" -ge "$min_n" ] && send "✅ freehire $(hostname): ${ok_text}"
		n=0
	fi
	count[$name]=$n
}

# --- site check ---
http_code=$(curl -sS -m 10 -o /dev/null -w '%{http_code}' "$URL" 2>/dev/null || echo "000")
log "site $URL -> HTTP $http_code"
site_bad=0
[ "$http_code" != "200" ] && site_bad=1
check site "$site_bad" "site down: ${URL} returned HTTP ${http_code}" "site back up: ${URL} returned HTTP 200"

# --- real-endpoint check ---
# Why /health is not enough: it is a trivial handler, so it answers 200 in
# milliseconds under conditions where the site is unusable. On 2026-08-16 it
# stayed green for hours while /api/v1/jobs took 61s and Cloudflare turned that
# into 520s for every visitor. This check requests what the site actually needs
# and treats slow as down — curl's -m turns a timeout into code 000, so the
# deadline needs no separate comparison.
#
# It goes through Cloudflare deliberately. That covers one more failure mode than
# a loopback probe (an edge or DNS fault the box itself cannot see) at the cost
# of one: an outage of Cloudflare alone will page us. That trade is right for a
# site whose visitors all arrive through the edge.
api_read=$(curl -sS -m "$API_TIMEOUT" -o /dev/null -w '%{http_code} %{time_total}' "$API_URL" 2>/dev/null || echo "000 ${API_TIMEOUT}")
api_code=${api_read%% *}
api_time=${api_read##* }
log "api ${API_URL} -> HTTP ${api_code} in ${api_time}s (timeout ${API_TIMEOUT}s)"
api_bad=0
[ "$api_code" != "200" ] && api_bad=1
check api "$api_bad" \
	"API failing: ${API_URL} returned HTTP ${api_code} after ${api_time}s (000 = no answer within ${API_TIMEOUT}s). /health may still be green — it is too cheap to prove the site works" \
	"API back: ${API_URL} returned HTTP 200 in ${api_time}s" \
	2

# --- memory check ---
mem_total=$(awk '/^MemTotal:/{print $2}' /proc/meminfo)
mem_avail=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo)
free_pct=$((mem_avail * 100 / mem_total))
log "memory ${free_pct}% available (threshold ${MIN_FREE_PCT}%)"
mem_bad=0
[ "$free_pct" -lt "$MIN_FREE_PCT" ] && mem_bad=1
check mem "$mem_bad" "low memory: only ${free_pct}% RAM available" "memory back to normal: ${free_pct}% RAM available"

# --- load check ---
# The blind spot this closes: /health is a trivial handler, so it answers 200
# long after the box has run out of CPU to serve real traffic. On 2026-08-16 a
# forgotten embed backfill held a locally-run TEI container at ~7 of 16 cores
# for two days; /health stayed green while /api/v1/jobs took 61s and Cloudflare
# returned 520. Load average is the signal that saw it — coarsely; see
# MAX_LOAD_PER_CORE_X10 for why it is set where it is.
#
# Compared in hundredths because /proc/loadavg is fractional and bash has no
# floats. Alerts only after 2 consecutive runs (~4 min) — see check()'s min_n.
cores=$(nproc)
load1_x100=$(awk '{printf "%d", $1 * 100}' /proc/loadavg)
load_limit_x100=$((cores * MAX_LOAD_PER_CORE_X10 * 10))
load1=$(awk '{print $1}' /proc/loadavg)
log "load ${load1} over ${cores} cores (threshold $(awk -v c="$cores" -v m="$MAX_LOAD_PER_CORE_X10" 'BEGIN{printf "%.1f", c * m / 10}'))"
load_bad=0
[ "$load1_x100" -ge "$load_limit_x100" ] && load_bad=1
check load "$load_bad" \
	"CPU starvation: load ${load1} on ${cores} cores — the site can still answer /health while real requests time out; check for a stray backfill (systemd-run units, docker ps)" \
	"load back to normal: ${load1} on ${cores} cores" \
	2

{
	for k in "${!count[@]}"; do printf '%s=%s ' "$k" "${count[$k]}"; done
	printf '\n'
} >"$STATE_FILE"

exit 0
