#!/bin/sh
# Per-checkout development environment, sourced by mise (.mise.toml [env] _.source).
#
# Every checkout of this repository (the primary one and each linked worktree) shares one
# local stack: the docker-compose services under the fixed project name `hire`, so one
# Postgres on :5432. What must NOT be shared is the database and the API port, or two agents
# working in two worktrees migrate and serve over each other. So each checkout gets:
#
#   - its own database on that server: hire_dev in the primary checkout, hire_wt_<n> in a
#     worktree, where <n> is a checksum of the worktree's path (bb names every worktree
#     directory `hire`, so the directory name alone cannot tell them apart);
#   - its own API port: 8080 in the primary checkout, 18000 + <n> % 1000 in a worktree.
#
# `mise run db:up` starts the stack and creates and migrates this checkout's database.

root=$(git rev-parse --show-toplevel 2>/dev/null) || return 0

if [ "$(git rev-parse --path-format=absolute --git-dir)" = "$(git rev-parse --path-format=absolute --git-common-dir)" ]; then
  db=hire_dev
  port=8080
else
  n=$(printf '%s' "$root" | cksum | cut -d' ' -f1)
  db="hire_wt_$n"
  # shortcut: two worktrees can land on one port (1 in 1000 per pair); pick a free slot
  # instead once that actually happens.
  port=$((18000 + n % 1000))
fi

# docker compose reads DB_HOST_PORT from .env too, so the URLs below must.
if [ -z "${DB_HOST_PORT:-}" ] && [ -f "$root/.env" ]; then
  # The digits only: compose strips quotes and a trailing comment from the value.
  DB_HOST_PORT=$(sed -n 's/^DB_HOST_PORT=["'"'"']\{0,1\}\([0-9]*\).*/\1/p' "$root/.env" | tail -n 1)
fi

export DB_HOST_PORT="${DB_HOST_PORT:-5432}"
export COMPOSE_PROJECT_NAME=hire
export HIRE_DB_NAME="$db"
export PORT="$port"
export DATABASE_URL="postgres://hire:hire@localhost:$DB_HOST_PORT/$db?sslmode=disable"
# The web dev server proxies /api to this checkout's server (web/vite.config.ts).
export VITE_API_URL="http://localhost:$port"
# Integration tests migrate their template on this server instead of starting a container
# per test binary (internal/platform/testdb); `mise run db:up` must have started it.
export TESTDB_ADMIN_URL="postgres://hire:hire@localhost:$DB_HOST_PORT/postgres?sslmode=disable"
