#!/usr/bin/env bash
# Fails when this change breaks a client of the public API, as described by
# web/static/openapi.yaml — the contract served at freehire.me/openapi.yaml, which paid
# API keys are now sold against.
#
# "Breaking" is oasdiff's verdict at ERR level: a removed endpoint or field, a request
# bound that rejects values the old contract accepted, a response property that promises
# less than it did. Measured on the 30 changes to the file before this check existed, it
# fired on 5, and all 5 were real — #3420 dropped `format: uri` from every job `url`,
# #2815 capped `offset` at 10000.
#
# Some breaks are deliberate. Say so in a commit trailer and the check passes, printing
# the reason, so it lives in the history beside the change rather than nowhere:
#
#   API-Break: <reason>
#
# Usage: scripts/check-openapi-breaking.sh   (OASDIFF names the binary; default: oasdiff)
set -euo pipefail

SPEC=web/static/openapi.yaml
OASDIFF=${OASDIFF:-oasdiff}

# An unresolvable base is a broken assumption, not "nothing changed" — a shallow clone
# would otherwise compare the file with itself and pass having examined nothing.
base=""
for ref in origin/main main; do
  if base=$(git merge-base "$ref" HEAD 2>/dev/null); then break; fi
  base=""
done
if [ -z "$base" ]; then
  echo "check-openapi-breaking: no merge base against origin/main or main." >&2
  echo "Fetch the main branch (CI needs fetch-depth: 0)." >&2
  exit 1
fi

if git diff --quiet "$base" HEAD -- "$SPEC"; then
  echo "check-openapi-breaking: $SPEC unchanged."
  exit 0
fi

old=$(mktemp)
trap 'rm -f "$old"' EXIT
git show "$base:$SPEC" > "$old"

if "$OASDIFF" breaking "$old" "$SPEC" --fail-on ERR -f singleline; then
  echo "check-openapi-breaking: no breaking change."
  exit 0
fi

reasons=$(git log --format='%(trailers:key=API-Break,valueonly,separator=%x0A)' "$base..HEAD" | sed -e '/^[[:space:]]*$/d' -e 's/^/  /')
if [ -n "$reasons" ]; then
  echo
  echo "Allowed by an API-Break trailer:"
  echo "$reasons"
  exit 0
fi

cat >&2 <<'EOF'

A client written against the current contract would break on the change above. Keep the
old shape (add, don't remove; widen, don't narrow), or — if the break is intended —
say why in a commit trailer:

  API-Break: <reason>
EOF
exit 1
