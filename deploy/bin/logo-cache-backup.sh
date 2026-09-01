#!/usr/bin/env bash
# Nightly mirror of the logo cache to Hetzner Object Storage (host-2).
#
# `sync`, not a tar: cache entries are immutable, so a steady-state night uploads only
# the logos resolved that day and moves almost nothing.
#
# This protects the logo.dev ALLOWANCE, not the data. The logos are re-derivable, just not
# affordably all at once: a lost cache disk means ~294,000 upstream fetches, which is a
# monthly cap spent in one restore.
#
# Installed at /opt/freehire/bin/logo-cache-backup.sh, driven by
# freehire-logo-cache-backup.timer. Fails loudly (set -e) so a broken run shows up in
# `systemctl --failed`.
set -euo pipefail

CACHE_DIR=${CACHE_DIR:-/var/cache/freehire-logo}
S3_REMOTE=hz
BACKUP_BUCKET=freehire-backups
S3_PREFIX=logos

# S3 credentials and the rclone remote, exactly as pg-backup.sh does it: read from the
# 0600 env file and exported as RCLONE_CONFIG_HZ_* so no rclone.conf exists on disk.
#
# The exports are not optional decoration. Without them `hz` is an unknown remote and
# EVERY run fails — which is loud, thanks to set -e, but leaves the cache this script
# exists to protect unbacked-up.
set -a
# The env file is deployment state, not a checked-in script: shellcheck can neither
# find it here nor learn anything from it there.
# shellcheck disable=SC1090,SC1091
. /opt/freehire/.env
set +a
: "${S3_ENDPOINT:?S3_ENDPOINT missing from /opt/freehire/.env}"
: "${S3_ACCESS_KEY:?S3_ACCESS_KEY missing from /opt/freehire/.env}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY missing from /opt/freehire/.env}"
# Region is the first endpoint label (hel1.your-objectstorage.com -> hel1); Hetzner
# rejects writes if the location constraint disagrees.
S3_REGION=$(printf '%s' "$S3_ENDPOINT" | sed -E 's#^https?://([^.]+)\..*#\1#')
export RCLONE_CONFIG_HZ_TYPE=s3
export RCLONE_CONFIG_HZ_PROVIDER=Other
export RCLONE_CONFIG_HZ_ENDPOINT="$S3_ENDPOINT"
export RCLONE_CONFIG_HZ_REGION="$S3_REGION"
export RCLONE_CONFIG_HZ_LOCATION_CONSTRAINT="$S3_REGION"
export RCLONE_CONFIG_HZ_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export RCLONE_CONFIG_HZ_SECRET_ACCESS_KEY="$S3_SECRET_KEY"

if [ ! -d "$CACHE_DIR" ]; then
  echo "logo-cache-backup: $CACHE_DIR does not exist" >&2
  exit 1
fi

before=$(find "$CACHE_DIR" -type f | wc -l)
rclone sync "$CACHE_DIR" "${S3_REMOTE}:${BACKUP_BUCKET}/${S3_PREFIX}" \
  --transfers 16 --checkers 32 --stats-one-line
echo "logo-cache-backup: mirrored $before entries to ${BACKUP_BUCKET}/${S3_PREFIX}"
