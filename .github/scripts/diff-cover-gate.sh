#!/usr/bin/env bash
# Fail when the lines a change touches are covered by tests below a threshold.
#
#   diff-cover-gate.sh go  <dir with the shards' *.out Go profiles>
#   diff-cover-gate.sh web <vitest lcov file>
#
# Run from anywhere inside the repository. Needs origin/main to be fetched deep enough to
# diff against, and python3 with the venv module. COVERAGE_FAIL_UNDER overrides a threshold
# for one run (0 measures without gating).
set -euo pipefail

# The thresholds. Raising or lowering one is a reviewed change to this file. Web is 0: it
# reports the percentage and gates nothing until a threshold is chosen from enough merged
# pull requests (AGENTS.md says why).
GO_FAIL_UNDER=${COVERAGE_FAIL_UNDER:-60}
WEB_FAIL_UNDER=${COVERAGE_FAIL_UNDER:-0}

kind=${1:?usage: diff-cover-gate.sh go|web <input>}
input=${2:?usage: diff-cover-gate.sh go|web <input>}

root=$(git rev-parse --show-toplevel)
here=$(CDPATH='' cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# A hash-pinned install into a throwaway virtualenv; never a bare `pip install`.
if ! command -v diff-cover >/dev/null 2>&1; then
  python3 -m venv "$work/venv"
  "$work/venv/bin/pip" install --quiet --require-hashes -r "$here/../requirements/diff-cover.txt"
  export PATH="$work/venv/bin:$PATH"
fi

case "$kind" in
  go)
    shopt -s nullglob
    # Absolute, because the script changes to the repository root below.
    dir=$(CDPATH='' cd -- "$input" && pwd)
    profiles=("$dir"/*.out)
    if [ "${#profiles[@]}" -eq 0 ]; then
      echo "No Go coverage profiles in $dir" >&2
      exit 1
    fi
    reports=()
    cd "$root"
    for profile in "${profiles[@]}"; do
      report="$work/$(basename "$profile" .out).xml"
      go run github.com/boumenot/gocover-cobertura@v1.5.0 \
        -ignore-gen-files -ignore-dirs 'internal/platform/db' \
        <"$profile" >"$report"
      reports+=("$report")
    done
    # Several reports are merged by diff-cover: a line covered in any shard is covered.
    # Generated code is dropped by the converter above; command entry points here.
    diff-cover "${reports[@]}" \
      --exclude 'cmd/*/main.go' 'internal/platform/db/*' \
      --compare-branch=origin/main --fail-under="$GO_FAIL_UNDER"
    ;;
  web)
    # vitest writes paths relative to web/; diff-cover compares them with git's repository-
    # relative ones, so the prefix is added to a copy of the report.
    lcov="$work/lcov.info"
    sed 's#^SF:src/#SF:web/src/#' "$input" >"$lcov"
    cd "$root"
    diff-cover "$lcov" --exclude 'web/src/*generated/*' \
      --compare-branch=origin/main --fail-under="$WEB_FAIL_UNDER"
    ;;
  *)
    echo "unknown kind: $kind" >&2
    exit 1
    ;;
esac
