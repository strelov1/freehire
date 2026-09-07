#!/usr/bin/env bash
# Commits a refreshed contributor snapshot onto the branch the pull request tracks.
# Called by .github/workflows/contributors.yml, and only when the file actually changed.
#
# The branch is recreated from the checked-out main on every run rather than added to,
# so the pull request is always one commit against current main — a chain of daily
# "refresh" commits would be a worse record of the same single fact. That is what makes
# the force-push safe and necessary: nothing but this job ever writes this branch, and
# what it force-pushes over is its own superseded version of the same file.
set -euo pipefail

: "${BRANCH:?BRANCH must be set}"
: "${SNAPSHOT:?SNAPSHOT must be set}"

git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'

git checkout -B "$BRANCH"
git add "$SNAPSHOT"
git commit -m 'Refresh the contributor snapshot'
git push --force origin "$BRANCH"
