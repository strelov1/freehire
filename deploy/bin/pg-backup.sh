#!/usr/bin/env bash
# Nightly PostgreSQL backup for freehire (host-2).
#
# Dumps every database in DATABASES with pg_dump custom format (-Fc), verifies
# each archive is readable, uploads it to Hetzner Object Storage, then prunes
# old copies (local: keep the newest LOCAL_KEEP per db; S3: drop objects older
# than S3_MAX_AGE).
#
# Runs as root:
#   - pg_dump connects as the postgres OS user (peer auth, no password),
#   - S3 credentials come from /opt/freehire/.env (already 0600),
#   - rclone is configured entirely from env vars (no persistent rclone.conf).
#
# Installed at /opt/freehire/bin/pg-backup.sh, driven by
# freehire-pg-backup.timer at 03:00 UTC. Fails loudly (set -e) so a broken run
# surfaces in `systemctl --failed`.
set -euo pipefail

# `mail` was the retired apply app's DB (dropped 2026-07-15); only `hire` remains.
DATABASES=(hire)
BACKUP_DIR=/var/backups/freehire
S3_REMOTE=hz
BACKUP_BUCKET=freehire-backups
S3_PREFIX=pg
# Keep only the newest local dump — S3 (below) is the authoritative backup with
# S3_MAX_AGE history. On host-2's 301G disk, Meili (~128G) + PG (~70G) + a facet
# reindex's ~2x transient index leave no room for a stack of ~19G dumps; three of
# them once filled the disk mid-reindex (2026-07-20), aborting it on `unexpected
# EOF`. One local copy is the fast-restore convenience; older history lives in S3.
LOCAL_KEEP=1
S3_MAX_AGE=30d
ENV_FILE=/opt/freehire/.env

log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*"; }

# --- S3 credentials + rclone env-config (no rclone.conf on disk) ------------
# NOTE: .env defines its own S3_BUCKET (freehire-resumes) — that is why our
# target is BACKUP_BUCKET, a distinct name sourcing cannot clobber.
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
: "${S3_ENDPOINT:?S3_ENDPOINT missing from $ENV_FILE}"
: "${S3_ACCESS_KEY:?S3_ACCESS_KEY missing from $ENV_FILE}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY missing from $ENV_FILE}"
# Region is the first endpoint label (hel1.your-objectstorage.com -> hel1);
# Hetzner rejects CreateBucket/writes if the location constraint disagrees.
S3_REGION=$(printf '%s' "$S3_ENDPOINT" | sed -E 's#^https?://([^.]+)\..*#\1#')
export RCLONE_CONFIG_HZ_TYPE=s3
export RCLONE_CONFIG_HZ_PROVIDER=Other
export RCLONE_CONFIG_HZ_ENDPOINT="$S3_ENDPOINT"
export RCLONE_CONFIG_HZ_REGION="$S3_REGION"
export RCLONE_CONFIG_HZ_LOCATION_CONSTRAINT="$S3_REGION"
export RCLONE_CONFIG_HZ_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export RCLONE_CONFIG_HZ_SECRET_ACCESS_KEY="$S3_SECRET_KEY"

install -d -m 0700 "$BACKUP_DIR"
ts=$(date -u +%Y%m%dT%H%M%SZ)

for db in "${DATABASES[@]}"; do
  file="$BACKUP_DIR/${db}_${ts}.dump"
  log "dumping $db -> $file"
  # pg_dump runs as postgres (peer auth); root's shell owns the output file.
  #
  # UNTHROTTLED since 2026-08-20, deliberately reversing the previous nice -n 19 /
  # ionice -c 3 (idle class). Idle-class I/O measured 5.4 MB/s and stretched the run
  # to ~75 minutes; the throttle did not prevent the 2026-08-20 outage, it LENGTHENED
  # the window in which the dump competed with autovacuum on a 96 GB jobs table. A
  # short sharp run beats a long quiet one when the thing it collides with is another
  # whole-table reader. The unit pauses the ingest fleet for the duration, which is
  # what buys the headroom this used to buy by going slowly.
  #
  # What is still NOT throttled either way: the server-side sequential read runs in
  # the postgres backend, not here, so client priority never governed it. The real
  # fix remains a physical backup (pgBackRest/WAL), deferred.
  #
  # If this run's own dump/verify fails, remove its partial file immediately —
  # unconditional, independent of the upload step below. Previously `set -e`
  # exited before the retention step ever ran, so a failing dump (e.g. the
  # 2026-08-11 TOAST corruption incident) left an ever-growing pile of large
  # partial .dump files across multiple failed nights instead of just one.
  trap 'rm -f -- "$file"' ERR
  # zstd, not the -Fc default of gzip, and this is the change that actually made the
  # run fast. Measured 2026-08-20 on this host: gzip pinned pg_dump at 98.5% of ONE
  # core while I/O pressure sat at 3% — the dump was compression-bound, not
  # disk-bound, the whole time. Removing the nice/ionice throttle the same night
  # bought 5.4 -> 7.3 MB/s, because it was rationing a resource that was never
  # scarce. zstd alone then took it to 24 MB/s, a 4.4x win over the throttled gzip.
  #
  # NO workers= here. pg_dump answers `compression option "workers" is not currently
  # supported by pg_dump` and ignores it — the option exists in the shared compression
  # parser, not in this tool. An earlier revision passed workers=4 and the warning was
  # missed because the exit code was 0: the same mistake as trusting `pg_restore -l` on
  # a truncated archive, reading the wrong signal for success. The speed was always
  # single-threaded zstd.
  #
  # Requires PostgreSQL 16+ on BOTH sides: a zstd archive cannot be read by an older
  # pg_restore. This host runs 18 and the S3 copy is restored by the same major, so
  # the constraint is recorded rather than defended against.
  # --- API watchdog -------------------------------------------------------------
  #
  # Suspend the dump whenever the live API stops answering; resume when it recovers.
  # Better no backup tonight than a site that is down: a dump can be re-run at any
  # hour, an outage cannot be un-served.
  #
  # WHY THE PROBE IS THE API AND NOT CPU/IO. On 2026-08-20 the site 500ed while CPU
  # pressure read 0.00% full, I/O 0.87% full and load average 3.17 on 16 cores — the
  # hardware was idle. The bottleneck was the API's connection pool: pgx defaults to
  # roughly one connection per core, so once the dump made some queries slow, all ~16
  # were held and EVERY request queued behind them into a 60s timeout. Resource gauges
  # cannot see that. Whether the API answers is the only signal that can.
  #
  # WHY SIGSTOP AND NOT KILL. Killing pg_dump discards the whole run AND releases its
  # transaction snapshot, which lets autovacuum immediately attack the backlog it was
  # holding back; that burst is itself an outage on a 96 GB table (observed twice the
  # same night). STOP/CONT leaves both the work and the snapshot exactly where they are.
  #
  # WHY A PAUSE BUDGET. A stopped pg_dump still holds its snapshot, so autovacuum
  # cannot clean while it sits there — pausing forever trades a short outage for a
  # slow-growing one. Past the budget the run gives up, removes its partial file via
  # the ERR trap, and exits non-zero so the missing backup is visible rather than
  # silently absent.
  api_port=$(grep -oE 'freehire_api \{ server 127\.0\.0\.1:[0-9]+' /etc/nginx/snippets/freehire-upstream-active.conf | grep -oE '[0-9]+$')
  probe_url="http://127.0.0.1:${api_port}/api/v1/jobs?limit=1"
  PAUSE_BUDGET=900
  watch_api() {
    local dump_pid=$1 fails=0 paused=0 paused_total=0
    while kill -0 "$dump_pid" 2>/dev/null; do
      sleep 10
      if curl -fsS --max-time 8 -o /dev/null "$probe_url" 2>/dev/null; then
        fails=0
        if [ "$paused" = 1 ]; then
          kill -CONT "$dump_pid" 2>/dev/null && log "api recovered, resuming dump"
          paused=0
        fi
      else
        fails=$((fails + 1))
        # Two consecutive misses, not one: a single slow probe is noise, and
        # site-alert.sh uses the same two-strike rule for the same reason.
        if [ "$fails" -ge 2 ] && [ "$paused" = 0 ]; then
          kill -STOP "$dump_pid" 2>/dev/null && log "api not answering, dump SUSPENDED"
          paused=1
        fi
        if [ "$paused" = 1 ]; then
          paused_total=$((paused_total + 10))
          if [ "$paused_total" -ge "$PAUSE_BUDGET" ]; then
            log "api still down after ${paused_total}s suspended, abandoning this run"
            kill -CONT "$dump_pid" 2>/dev/null
            kill -TERM "$dump_pid" 2>/dev/null
            return 1
          fi
        fi
      fi
    done
  }
  runuser -u postgres -- pg_dump -Fc --compress=zstd:3 "$db" > "$file" &
  dump_pid=$!
  watch_api "$dump_pid" &
  watcher_pid=$!
  wait "$dump_pid"; dump_rc=$?
  kill "$watcher_pid" 2>/dev/null || true
  if [ "$dump_rc" -ne 0 ]; then log "dump failed rc=$dump_rc"; rm -f -- "$file"; exit 1; fi

  # Fail the run if the archive's table of contents is not readable.
  pg_restore -l "$file" > /dev/null
  log "verified $db dump ($(du -h "$file" | cut -f1))"
  trap - ERR

  # Local retention: keep the newest LOCAL_KEEP *verified* dumps for this db.
  # Runs right after verification, before the S3 upload — an upload outage no
  # longer stops local disk usage from being bounded.
  # ls -t, not find: the retention rule is "the newest N", so the sort by mtime IS
  # the operation. The names are this script's own timestamped dumps.
  # shellcheck disable=SC2012
  ls -1t "$BACKUP_DIR/${db}_"*.dump 2>/dev/null | tail -n +$((LOCAL_KEEP + 1)) \
    | xargs -r rm -f --

  dest="${S3_REMOTE}:${BACKUP_BUCKET}/${S3_PREFIX}/${db}/${db}_${ts}.dump"
  log "uploading -> $dest"
  rclone copyto "$file" "$dest"

  # S3 retention: drop objects older than S3_MAX_AGE.
  rclone delete --min-age "$S3_MAX_AGE" \
    "${S3_REMOTE}:${BACKUP_BUCKET}/${S3_PREFIX}/${db}/"
done

log "backup complete"
