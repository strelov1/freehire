#!/usr/bin/env bash
# Emit one disjoint part of the integration package list. Unclassified packages
# belong to rest so a new package cannot silently disappear from CI.
set -euo pipefail

shard=${1:?An integration shard is required}
case "$shard" in
  integration-handler | integration-db | integration-cmd | integration-rest) ;;
  *) echo "Unknown integration shard: $shard" >&2; exit 1 ;;
esac

module=$(go list -m)
go list -tags=integration ./... | while IFS= read -r package; do
  case "$package" in
    "$module/internal/api/handler") group=integration-handler ;;
    "$module/internal/platform/db") group=integration-db ;;
    "$module/cmd/"*) group=integration-cmd ;;
    *) group=integration-rest ;;
  esac
  if [[ "$group" == "$shard" ]]; then
    printf '%s\n' "$package"
  fi
done
