#!/usr/bin/env bash
# Reports where the production host and deploy/ have drifted apart.
#
# Nothing in deploy/ deploys itself, so an edit that never reached the host — or a
# hotfix applied on the host and never committed — is invisible until something
# breaks. This makes it one command:
#
#   ./deploy/check-drift.sh              # against the default host
#   DEPLOY_HOST=root@1.2.3.4 ./deploy/check-drift.sh
#
# Read-only on both sides. Exits 1 when anything differs, so it can gate a check.
set -euo pipefail

HOST=${DEPLOY_HOST:-root@89.167.94.146}
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# The same two sets deploy/ holds: units without the host's .bak clutter, and the
# shell scripts without the compiled binaries beside them.
ssh -o ConnectTimeout=15 "$HOST" '
  cd /etc/systemd/system && tar cz $(ls -d freehire-* | grep -v "\.bak") 2>/dev/null
' > "$work/units.tgz"
ssh -o ConnectTimeout=15 "$HOST" '
  cd /opt/freehire/bin && tar cz $(ls *.sh | grep -v "\.bak")
' > "$work/bin.tgz"

mkdir -p "$work/systemd" "$work/bin"
tar xzf "$work/units.tgz" -C "$work/systemd"
tar xzf "$work/bin.tgz" -C "$work/bin"

status=0
for set in systemd bin; do
  if diff -ru "$here/$set" "$work/$set" > "$work/$set.diff" 2>&1; then
    echo "$set: in sync"
  else
    echo "$set: DRIFTED"
    sed 's/^/  /' "$work/$set.diff"
    status=1
  fi
done

if [ "$status" -ne 0 ]; then
  echo
  echo "A '-' line is in git and not on the host; a '+' line is on the host and not in git."
fi
exit "$status"
