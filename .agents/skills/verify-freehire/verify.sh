#!/usr/bin/env bash
# Keep the stack and its journeys inside one foreground tool session.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
cd "$(git -C "$here" rev-parse --show-toplevel)"
if command -v mise >/dev/null 2>&1; then eval "$(mise env -s bash)"; fi
for tool in go pnpm psql curl python3; do
  command -v "$tool" >/dev/null || { printf 'Missing prerequisite: %s\n' "$tool" >&2; exit 1; }
done
if [ "${1:-}" = preflight ]; then exit 0; fi
if [ "$#" -eq 0 ]; then
  printf 'Usage: verify.sh preflight | <journey.spec.ts> [...]\n' >&2
  exit 2
fi
# Export one evidence directory for startup logs and every journey in this run.
export VERIFY_EVIDENCE
VERIFY_EVIDENCE=$(mktemp -d "${TMPDIR:-/tmp}/verify-freehire-run-XXXXXX")
eval "$("$here/stack.sh" env)"
# Refuse an existing instance before installing teardown ownership.
if ! mkdir "$VERIFY_STATE"; then
  printf 'State already exists: %s; inspect it before cleanup\n' "$VERIFY_STATE" >&2
  exit 1
fi
active_pid=
stop_tree() {
  local child
  for child in $(pgrep -P "$1" 2>/dev/null); do stop_tree "$child"; done
  kill "$1" 2>/dev/null || true
}
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [ -n "$active_pid" ]; then
    stop_tree "$active_pid"
    wait "$active_pid" 2>/dev/null || true
  fi
  mkdir -p "$VERIFY_EVIDENCE/diagnostics"
  if [ -d "$VERIFY_STATE" ]; then
    find "$VERIFY_STATE" -maxdepth 1 -type f -name '*.log' -exec cp {} "$VERIFY_EVIDENCE/diagnostics/" \; || true
  fi
  "$here/stack.sh" down >>"$VERIFY_EVIDENCE/cleanup.log" 2>&1 || true
  printf 'Evidence preserved: %s\n' "$VERIFY_EVIDENCE" >&2
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
run_child() {
  "$@" &
  active_pid=$!
  wait "$active_pid"
  active_pid=
}
run_child "$here/stack.sh" up >"$VERIFY_EVIDENCE/startup.log" 2>&1
run_child "$here/stack.sh" doctor >"$VERIFY_EVIDENCE/doctor.log" 2>&1
cat "$VERIFY_EVIDENCE/doctor.log"
for spec in "$@"; do run_child "$here/run-journey.sh" "$spec"; done
