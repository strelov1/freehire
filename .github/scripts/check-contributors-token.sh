#!/usr/bin/env bash
# Asks, before any work is done, whether CONTRIBUTORS_TOKEN can actually write to this
# repository — and says what to fix when it cannot.
#
# This exists because the failure it replaces was unreadable. A token missing one
# permission got all the way to `git push`, after thirty API calls of collection, and
# came back
#
#     remote: Permission to <owner>/<repo>.git denied to <user>.
#     fatal: ... The requested URL returned error: 403
#
# which names the user rather than the missing permission, and reads like the account is
# wrong when the account is fine.
#
# IT PROBES A REAL WRITE, AND THAT IS THE WHOLE POINT. The obvious check —
# `repos/{owner}/{repo}` and its `.permissions.push` — is a trap: that field reports the
# ROLE OF THE USER the token belongs to, not what the token was granted. A read-only
# fine-grained token held by an admin reports `push: true` and then cannot push, so the
# check would print a confident green and the job would fail anyway. A false green is
# worse than no check. Creating a ref and deleting it is the only answer that comes from
# the token's own permissions.
#
# The probe is safe: the branch exists for one API call, and creating a branch triggers
# no workflows here — CI fires on pushes to main and on pull requests, neither of which
# this is.
set -uo pipefail

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"

PROBE_REF='refs/heads/contributors-token-write-probe'

explain() {
  echo "CONTRIBUTORS_TOKEN cannot write to ${GITHUB_REPOSITORY}."
  echo
  echo 'A fine-grained token needs all three:'
  echo '  Repository access ....... Only select repositories, including this one'
  echo '  Contents ................ Read and write   <- the one this usually is'
  echo '  Pull requests ........... Read and write'
  echo
  echo 'A classic token needs the "repo" scope.'
  echo
  echo 'Note that the repository page reporting "push: true" for your account proves'
  echo 'nothing about the token: that is your role, not its grant.'
}

sha=$(gh api "repos/${GITHUB_REPOSITORY}/git/ref/heads/${GITHUB_REF_NAME:-main}" --jq '.object.sha' 2>/dev/null || true)
if [ -z "$sha" ]; then
  echo "CONTRIBUTORS_TOKEN cannot even read ${GITHUB_REPOSITORY} — wrong value, expired, or not granted access to this repository."
  echo
  explain
  exit 1
fi

if ! gh api -X POST "repos/${GITHUB_REPOSITORY}/git/refs" -f ref="$PROBE_REF" -f sha="$sha" >/dev/null 2>&1; then
  explain
  exit 1
fi

gh api -X DELETE "repos/${GITHUB_REPOSITORY}/git/${PROBE_REF}" >/dev/null 2>&1 || true
echo "CONTRIBUTORS_TOKEN can write to ${GITHUB_REPOSITORY}."
