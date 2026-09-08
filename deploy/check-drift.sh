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

# The same three sets deploy/ holds: units without the host's .bak clutter, the
# shell scripts without the compiled binaries beside them, and the nginx files that
# are hand-edited rather than generated.
#
# That last set used to be one file, described as "the one snippet that is hand-
# edited". It was not: freehire-api.conf holds every API location, including four
# hand-tuned SSE blocks and the proxy buffer sizing a 502 taught us, and
# freehire-logformat.conf holds the format two parsers read positionally. Both were
# edited by hand and tracked nowhere, which is the exact condition deploy/ exists to
# end. They are in now.
#
# freehire-upstream-active.conf stays out, and that one IS deliberate: it is the
# symlink the release flip repoints, so tracking it would report drift after every
# deploy and teach the reader to ignore this tool.
# One set: run a command on the host that writes a tar to stdout, unpack it under
# the directory of the same name here. Straight through the pipe rather than via a
# file, so an ssh that fails takes the pipeline with it instead of leaving an empty
# archive for tar to complain about.
fetch_set() {
  mkdir -p "$work/$1"
  ssh -o ConnectTimeout=15 "$HOST" "$2" | tar xz -C "$work/$1"
}

# shellcheck disable=SC2016  # single quotes on purpose: the $( ) picks the files on the HOST
fetch_set systemd 'cd /etc/systemd/system && tar cz $(ls -d freehire-* | grep -v "\.bak") 2>/dev/null'
# shellcheck disable=SC2016  # as above
fetch_set bin     'cd /opt/freehire/bin && tar cz $(ls *.sh | grep -v "\.bak")'
fetch_set nginx   'cd /etc/nginx && tar cz snippets/freehire-app.conf snippets/freehire-api.conf conf.d/freehire-logformat.conf'

status=0
for set in systemd bin nginx; do
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
