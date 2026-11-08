#!/usr/bin/env bash
# Runs one journey spec against the stack `stack.sh up` started, with the repo's own
# Playwright (web/node_modules, its pinned Chromium). The spec is copied into
# web/e2e/__verify__/ for the run (Playwright only runs specs under its testDir) and removed after.
#
#   run-journey.sh journeys/jobs-search.spec.ts        # or any spec path you wrote
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
spec=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
root=$(git -C "$here" rev-parse --show-toplevel)
eval "$("$here/stack.sh" env)"
mkdir -p "$VERIFY_EVIDENCE" "$root/web/e2e/__verify__"
name=$(basename "$spec" .spec.ts)
cp "$spec" "$root/web/e2e/__verify__/$name.spec.ts"
trap 'rm -rf "$root/web/e2e/__verify__"' EXIT
cd "$root/web"
# The trace (actions, network, DOM snapshots) lands next to the screenshots.
pnpm exec playwright test "e2e/__verify__/$name.spec.ts" --reporter=list --trace=on \
  --output="$VERIFY_EVIDENCE/$name-playwright" 2>&1 | tee "$VERIFY_EVIDENCE/$name.log"
exit "${PIPESTATUS[0]}"
