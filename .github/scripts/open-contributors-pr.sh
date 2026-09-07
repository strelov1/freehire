#!/usr/bin/env bash
# Opens the pull request carrying a refreshed contributor snapshot, or leaves the open
# one to pick up the push that just happened. Called by
# .github/workflows/contributors.yml after commit-contributors.sh.
#
# `gh pr create` fails outright when a pull request is already open for the branch, and
# that is the ordinary case rather than the exceptional one: a second day's change
# arrives before the first has merged. So the branch is asked about first.
#
# Auto-merge is (re-)requested either way. It is not a no-op on an existing pull
# request — a merge that was queued and then invalidated by a red run needs asking for
# again, and asking twice costs nothing.
set -euo pipefail

: "${BRANCH:?BRANCH must be set}"

if [ -z "$(gh pr list --head "$BRANCH" --state open --json number --jq '.[].number')" ]; then
  gh pr create \
    --base main \
    --head "$BRANCH" \
    --title 'Refresh the contributor snapshot' \
    --body 'Opened by the contributors workflow. The snapshot behind /contributors changed: someone merged a pull request, opened an issue, or contributed for the first time.

Nothing here is hand-written — the file is whatever web/scripts/build-contributors.mjs collected from the GitHub API. Reviewing it means checking the diff is people, not a shape change.'
fi

gh pr merge "$BRANCH" --auto --squash
