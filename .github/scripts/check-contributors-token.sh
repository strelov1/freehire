#!/usr/bin/env bash
# Asks, before any work is done, whether CONTRIBUTORS_TOKEN can actually write to this
# repository — and says what to fix when it cannot.
#
# This exists because the failure it replaces was unreadable. A token missing one
# permission got all the way to `git push` and came back
#
#     remote: Permission to <owner>/<repo>.git denied to <user>.
#     fatal: ... The requested URL returned error: 403
#
# which names the user rather than the missing permission, and reads like the account
# is wrong when the account is fine. Everything about this job that can go wrong is a
# credential, so the credential is checked first and reported in words someone can act
# on. The repository's own permissions are what is read, so nothing about the token is
# printed.
set -euo pipefail

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"

# `|| true`: an unusable token exits non-zero here, and under `set -e` that would kill
# the script before it could explain itself — which is the whole failure this replaces.
push=$(gh api "repos/${GITHUB_REPOSITORY}" --jq '.permissions.push' 2>/dev/null || true)

if [ "$push" = "true" ]; then
  echo "CONTRIBUTORS_TOKEN can write to ${GITHUB_REPOSITORY}."
  exit 0
fi

echo "CONTRIBUTORS_TOKEN cannot write to ${GITHUB_REPOSITORY} (repository reports push=${push:-<no answer>})."
echo
echo 'An empty answer means the token was rejected outright — wrong value, expired, or'
echo 'not granted access to this repository. "false" means it was accepted and simply'
echo 'has no write permission.'
echo
echo 'A fine-grained token needs all three:'
echo '  Repository access ....... Only select repositories, including this one'
echo '  Contents ................ Read and write'
echo '  Pull requests ........... Read and write'
echo
echo 'A classic token needs the "repo" scope.'
exit 1
