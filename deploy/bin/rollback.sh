#!/usr/bin/env bash
# Instantly flip nginx back to the other (warm) color. Usage: rollback.sh [freehire|apply]
#
# The `apply` branch is provisioned ahead of the app itself: as of 2026-08-17
# host-2 has no freehire-apply-api@/freehire-apply-web@ units and no
# apply-upstream-*.conf snippets, so that argument fails at the first
# systemctl. Harmless — the default is freehire, whose behaviour is unchanged —
# but do not read the case arm as evidence that a second app is deployed.
set -euo pipefail
app="${1:-freehire}"
case "$app" in
  freehire) snip=freehire-upstream; apisvc=freehire-api;       websvc=freehire-web;       b_api=8081; g_api=8082 ;;
  apply)    snip=apply-upstream;    apisvc=freehire-apply-api; websvc=freehire-apply-web; b_api=8085; g_api=8086 ;;
  *) echo "usage: rollback.sh [freehire|apply]" >&2; exit 1 ;;
esac
S=/etc/nginx/snippets
active=$(readlink -f "$S/${snip}-active.conf")
case "$active" in
  *-blue.conf)  target=green; aport=$g_api ;;
  *-green.conf) target=blue;  aport=$b_api ;;
  *) echo "rollback: cannot determine active color" >&2; exit 1 ;;
esac
systemctl is-active "${apisvc}@${target}" >/dev/null 2>&1 || systemctl start "${apisvc}@${target}" "${websvc}@${target}"
curl -fsS "http://127.0.0.1:$aport/health" >/dev/null || { echo "rollback: target $target not healthy, aborting" >&2; exit 1; }
ln -sf "$S/${snip}-${target}.conf" "$S/${snip}-active.conf"
nginx -t && nginx -s reload
echo "rolled back $app to $target"
