#!/usr/bin/env bash
# Web-tier Prometheus metrics for freehire (host-2), via the node_exporter
# textfile collector.
#
# Prometheus scrapes node_exporter, Postgres, the Go API and Meilisearch. The SSR
# web tier — the only component that has actually fallen over — had nothing, so
# every incident was first noticed from the watchdog's Telegram message after it
# had already restarted the process.
#
# Two exports, both the evidence those incidents were reconstructed from:
#
#   1. Accept-queue depth per colour. On a LISTEN socket, ss(8) reports the queue
#      depth as Recv-Q and the configured backlog as Send-Q — not byte counts.
#      Depth reaching the backlog is the exact condition web-watchdog.sh restarts
#      on, and it lived only in that script's log. Both colours, because a
#      capacity experiment against the idle one is invisible otherwise.
#
#   2. Response rates by status class, from nginx's access log. 504 is broken out
#      of 5xx: here it means "timed out while CONNECTING to upstream", i.e. a full
#      accept queue, which says something else entirely than an application 500.
#
# Latency is absent on purpose. It needs $upstream_response_time in log_format,
# and internal/viewlog parses this log's combined format to count job views — not
# worth breaking that for. Latency comes from perf/k6.
#
# Rates are gauges over the interval since the last run, not counters: a textfile
# counter must stay monotonic across reboots, log rotations and this script's own
# restarts, and a recomputed rate carries the same signal without that state.
#
# Installed at /opt/freehire/bin/web-metrics.sh, driven by
# freehire-web-metrics.timer (every 15s, matching the watchdog it explains).
set -uo pipefail

S=/etc/nginx/snippets
LOG=/var/log/nginx/access.log
TEXTFILE_DIR=/var/lib/prometheus/node-exporter
STATE_FILE=/run/freehire-web-metrics.state
OUT="$TEXTFILE_DIR/web.prom"

log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*"; }

[ -d "$TEXTFILE_DIR" ] || {
	log "textfile dir $TEXTFILE_DIR missing — is node_exporter installed?"
	exit 0
}

active=$(readlink -f "$S/freehire-upstream-active.conf" 2>/dev/null || true)
case "$active" in
	*-blue.conf) active_color=blue ;;
	*-green.conf) active_color=green ;;
	*) active_color=unknown ;;
esac

now=$(date +%s)

# --- accept queues, both colours -------------------------------------------
queue_lines=""
for pair in "blue 8083" "green 8084"; do
	# Splitting the pair into two positional arguments is the point here.
	# shellcheck disable=SC2086
	set -- $pair
	color=$1 port=$2
	line=$(ss -ltn "sport = :${port}" 2>/dev/null | awk 'NR==2')
	# A colour with no LISTEN socket is not "queue 0" — it is not running, and a 0
	# would read as healthy. Emit nothing and let the series go stale.
	[ -z "$line" ] && continue
	recvq=$(echo "$line" | awk '{print $2}')
	sendq=$(echo "$line" | awk '{print $3}')
	is_active=0
	[ "$color" = "$active_color" ] && is_active=1
	queue_lines="${queue_lines}freehire_web_accept_queue{color=\"${color}\"} ${recvq:-0}
freehire_web_accept_queue_limit{color=\"${color}\"} ${sendq:-0}
freehire_web_active{color=\"${color}\"} ${is_active}
"
done

# --- response rates from the access log -------------------------------------
# Only the bytes appended since the last run: the log grows by millions of lines
# a day, so re-reading it would cost more than everything else here combined.
prev=$(cat "$STATE_FILE" 2>/dev/null || echo "offset=0 ts=0")
offset=$(echo "$prev" | grep -oE 'offset=[0-9]+' | cut -d= -f2)
prev_ts=$(echo "$prev" | grep -oE 'ts=[0-9]+' | cut -d= -f2)
offset=${offset:-0}
prev_ts=${prev_ts:-0}

size=$(stat -c %s "$LOG" 2>/dev/null || echo 0)
# Shrunk since last run ⇒ logrotate moved the file underneath us. Start from the
# new file's beginning; losing the rotated tail beats double-counting it.
[ "$size" -lt "$offset" ] && offset=0

elapsed=$((now - prev_ts))
counts=""
latency=""
if [ "$prev_ts" -gt 0 ] && [ "$elapsed" -gt 0 ] && [ "$size" -gt "$offset" ]; then
	# The slice is read TWICE — once for rates, once for latency — so it is spooled
	# to a file rather than piped. It is one interval's worth of lines, a few hundred
	# KB at this site's rate, and re-running tail|head over the log would be the more
	# expensive of the two.
	slice=$(mktemp) || slice=""
	if [ -n "$slice" ]; then
		trap 'rm -f "$slice"' EXIT
		tail -c "+$((offset + 1))" "$LOG" 2>/dev/null | head -c "$((size - offset))" >"$slice"

		counts=$(awk -v elapsed="$elapsed" '
			# $9 is the status code in the combined log format. Requests still
			# in flight and malformed lines do not match, and are skipped.
			$9 ~ /^[0-9]{3}$/ {
				total++
				if ($9 == 504) { c["504"]++ }
				else if ($9 >= 500) { c["5xx"]++ }
				else if ($9 == 429) { c["429"]++ }
				else if ($9 >= 400) { c["4xx"]++ }
				else if ($9 >= 300) { c["3xx"]++ }
				else { c["2xx"]++ }
			}
			END {
				split("2xx 3xx 4xx 429 5xx 504", classes, " ")
				for (i in classes) {
					printf "freehire_nginx_responses_per_second{class=\"%s\"} %.3f\n", classes[i], c[classes[i]] / elapsed
				}
				printf "freehire_nginx_requests_per_second %.3f\n", total / elapsed
			}' "$slice")

		# --- latency ---------------------------------------------------------
		# This is the thing the file's own header used to say was not worth having:
		# "latency needs $upstream_response_time in log_format, and internal/viewlog
		# parses this log". The objection turned out to be untrue — viewlog's pattern
		# is anchored at ^ and not at $ — and is pinned by a test in that package.
		#
		# $request_time is read off the END of the line, not by field number. The
		# referer and user-agent are quoted but contain spaces, so counting forward
		# stops working after $body_bytes_sent, and $upstream_response_time before it
		# can be a comma-separated list, so counting backward past it does not work
		# either. The last field is the only one that can be addressed safely, which
		# is why log_format puts $request_time there.
		#
		# A line that does not end in a number is simply skipped, which is what makes
		# the deploy orderless: lines written before the format change carry no timing
		# and contribute nothing rather than a zero.
		#
		# Split api/page because they answer different questions and have different
		# costs. A page is an SSR render on ONE Node process and is what a visitor
		# means by "the site is slow"; /api/ is the Go service. /_app/ is dropped
		# entirely — hashed assets come off disk and would drag every percentile down
		# with numbers no human waits for.
		#
		# Percentiles over the interval, as gauges, for the same reason the rates
		# above are gauges: a textfile counter has to stay monotonic across reboots,
		# rotations and this script's own restarts, and there is no histogram here to
		# aggregate properly. These are honest for "how slow was the last 15 seconds"
		# and must not be averaged across a longer window as if they were quantiles
		# of a distribution.
		latency=$(awk '
			match($0, /[0-9]+\.[0-9]+$/) {
				path = $7
				if (path ~ /^\/_app\//) next
				print ((path ~ /^\/api\//) ? "api" : "page"), substr($0, RSTART, RLENGTH)
			}' "$slice" |
			sort -k1,1 -k2,2n |
			awk '
				{ n[$1]++; v[$1, n[$1]] = $2 }
				END {
					split("0.5 0.9 0.99", qs, " ")
					for (kind in n) {
						for (i in qs) {
							idx = int(n[kind] * (qs[i] + 0))
							if (idx < 1) idx = 1
							printf "freehire_nginx_request_seconds{kind=\"%s\",quantile=\"%s\"} %.3f\n", kind, qs[i], v[kind, idx]
						}
						printf "freehire_nginx_request_seconds{kind=\"%s\",quantile=\"1.0\"} %.3f\n", kind, v[kind, n[kind]]
						printf "freehire_nginx_request_samples{kind=\"%s\"} %d\n", kind, n[kind]
					}
				}')
	fi
fi

echo "offset=${size} ts=${now}" >"$STATE_FILE"

# --- emit --------------------------------------------------------------------
# Write-then-rename: the collector polls on its own schedule and would otherwise
# be able to read a half-written file — same reason internal/worker/metrics.go does.
tmp="${OUT}.tmp"
{
	echo "# HELP freehire_web_accept_queue Pending connections in the SSR listener's accept queue (ss Recv-Q on a LISTEN socket)."
	echo "# TYPE freehire_web_accept_queue gauge"
	echo "# HELP freehire_web_accept_queue_limit Configured backlog for that listener (ss Send-Q on a LISTEN socket)."
	echo "# TYPE freehire_web_accept_queue_limit gauge"
	echo "# HELP freehire_web_active Whether this colour is the one nginx currently proxies to."
	echo "# TYPE freehire_web_active gauge"
	printf '%s' "$queue_lines"
	if [ -n "$counts" ]; then
		echo "# HELP freehire_nginx_responses_per_second Responses per second by status class since the last scrape."
		echo "# TYPE freehire_nginx_responses_per_second gauge"
		echo "# HELP freehire_nginx_requests_per_second Total requests per second since the last scrape."
		echo "# TYPE freehire_nginx_requests_per_second gauge"
		printf '%s\n' "$counts"
	fi
	if [ -n "$latency" ]; then
		echo "# HELP freehire_nginx_request_seconds Request duration over the last interval, as seen by nginx. kind=page is an SSR render, kind=api is the Go service; hashed assets are excluded. Quantiles are over the interval only and must not be averaged across a longer window."
		echo "# TYPE freehire_nginx_request_seconds gauge"
		echo "# HELP freehire_nginx_request_samples How many requests the quantiles beside it were computed from, so a percentile off three samples can be told from one off three thousand."
		echo "# TYPE freehire_nginx_request_samples gauge"
		printf '%s\n' "$latency"
	fi
} >"$tmp" && mv "$tmp" "$OUT"

exit 0
