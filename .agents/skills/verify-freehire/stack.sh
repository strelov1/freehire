#!/usr/bin/env bash
# doctor's checks are single-quoted on purpose: eval expands them when they run.
# shellcheck disable=SC2016
# Starts, checks and tears down one isolated freehire stack for verification:
# its own Postgres database, its own Meilisearch, the Go API built from this checkout,
# the SvelteKit dev server in front of it, and one real board ingested as seed data.
#
#   stack.sh up       # start everything, seed, wait until /jobs renders postings
#   stack.sh doctor   # read-only: is this instance worth driving?
#   stack.sh env      # print the exports a drive needs (eval "$(stack.sh env)")
#   stack.sh down     # stop what `up` started, drop its database; evidence stays
#
# Runs on the laptop (mise + Docker) and in the SuperPlane coder runner (no Docker, a
# Postgres at TESTDB_ADMIN_URL, a `meilisearch` binary on PATH). See SKILL.md.
set -euo pipefail

SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

# mise's per-checkout env (PORT, TESTDB_ADMIN_URL) when it is there; the runner sets its own.
if command -v mise >/dev/null 2>&1 && [ -f .mise.toml ]; then
  eval "$(mise env -s bash 2>/dev/null)" || true
fi

API_PORT=${VERIFY_API_PORT:-${PORT:-18080}}
WEB_PORT=$((API_PORT + 1))
MEILI_PORT=$((API_PORT + 2))
STATE=${VERIFY_STATE:-/tmp/verify-freehire-$API_PORT}
EVIDENCE=${VERIFY_EVIDENCE:-/tmp/verify-freehire-evidence}
ADMIN_URL=${TESTDB_ADMIN_URL:?TESTDB_ADMIN_URL is unset: run inside the repo with mise, or in the coder runner}
DB_NAME=verify_$API_PORT
DATABASE_URL="${ADMIN_URL%%/postgres*}/$DB_NAME${ADMIN_URL#*/postgres}"
# Made per instance by `up` (a literal key here trips gitleaks), read back by the other verbs.
MEILI_KEY=$(cat "$STATE/meili-key" 2>/dev/null || true)
MEILI_CONTAINER=verify-freehire-meili-$API_PORT
SEED_PROVIDER=${VERIFY_SEED_PROVIDER:-greenhouse}
SEED_BOARD=${VERIFY_SEED_BOARD-vercel}  # set but empty = no seed

export DATABASE_URL MEILI_URL="http://localhost:$MEILI_PORT" MEILI_MASTER_KEY=$MEILI_KEY

# pnpm starts vite as a grandchild: stop the whole tree, children first.
killtree() {
  local c
  for c in $(pgrep -P "$1" 2>/dev/null); do killtree "$c"; done
  kill "$1" 2>/dev/null || true
}
log() { printf 'verify-freehire: %s\n' "$*" >&2; }
alive() { [ -f "$STATE/$1.pid" ] && kill -0 "$(cat "$STATE/$1.pid")" 2>/dev/null; }
wait_for() { # url, seconds, expected body substring (optional)
  local body
  for _ in $(seq 1 "$2"); do
    body=$(curl -fsS "$1" 2>/dev/null) && { [ -z "${3:-}" ] || [[ $body == *"$3"* ]]; } && return 0
    sleep 1
  done
  return 1
}

print_env() {
  cat <<EOF
export VERIFY_API_URL=http://localhost:$API_PORT
export VERIFY_WEB_URL=http://localhost:$WEB_PORT
export PLAYWRIGHT_BASE_URL=http://localhost:$WEB_PORT
export VERIFY_EVIDENCE=$EVIDENCE
export VERIFY_STATE=$STATE
export DATABASE_URL='$DATABASE_URL'
EOF
}

start_db() {
  if ! psql "$ADMIN_URL" -tAc 'SELECT 1' >/dev/null 2>&1; then
    command -v docker >/dev/null 2>&1 || { log "no Postgres at TESTDB_ADMIN_URL and no Docker to start one"; exit 1; }
    log "starting the shared dev Postgres (docker compose, service db)"
    docker compose up -d --wait --no-recreate db >/dev/null
  fi
  psql "$ADMIN_URL" -tAc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1 ||
    psql "$ADMIN_URL" -qc "CREATE DATABASE $DB_NAME" >/dev/null
  log "migrating $DB_NAME"
  go run ./cmd/migrate >"$STATE/migrate.log" 2>&1 || { tail -20 "$STATE/migrate.log" >&2; exit 1; }
}

start_meili() {
  mkdir -p "$STATE/meili"
  if command -v meilisearch >/dev/null 2>&1; then
    nohup meilisearch --http-addr "127.0.0.1:$MEILI_PORT" --master-key "$MEILI_KEY" --env development \
      --db-path "$STATE/meili/data.ms" --dump-dir "$STATE/meili/dumps" --no-analytics \
      </dev/null >"$STATE/meili.log" 2>&1 &
    echo $! >"$STATE/meili.pid"
  elif command -v docker >/dev/null 2>&1; then
    # Same image as docker-compose.yml, but a container of our own: the compose service is
    # shared by every checkout and its `jobs` index would be overwritten by ours.
    image=$(sed -n 's/^ *image: *\(getmeili\/meilisearch:.*\)$/\1/p' docker-compose.yml | head -1)
    docker rm -f "$MEILI_CONTAINER" >/dev/null 2>&1 || true
    docker run -d --name "$MEILI_CONTAINER" -p "127.0.0.1:$MEILI_PORT:7700" \
      -e MEILI_MASTER_KEY="$MEILI_KEY" -e MEILI_ENV=development "$image" >/dev/null
  else
    log "no meilisearch binary and no Docker: search pages (/jobs) will answer 500"
    return 0
  fi
  wait_for "$MEILI_URL/health" 60 available || { log "Meilisearch did not come up"; exit 1; }
}

seed() {
  [ -n "$SEED_BOARD" ] || return 0
  curl -fsS "$MEILI_URL/health" >/dev/null 2>&1 || return 0
  log "seeding: reindex (empty), add board $SEED_PROVIDER/$SEED_BOARD, ingest, drain to search"
  {
    go run ./cmd/reindex &&
      go run ./cmd/add-board --provider="$SEED_PROVIDER" --board="$SEED_BOARD" --company="$SEED_BOARD" --apply &&
      go run ./cmd/ingest "$SEED_PROVIDER" &&
      go run ./cmd/search-drain
  } >"$STATE/seed.log" 2>&1 || { tail -20 "$STATE/seed.log" >&2; exit 1; }
  grep -E 'ingest done|search-drain done' "$STATE/seed.log" >&2 || true
}

start_api() {
  log "building and starting the API on :$API_PORT"
  go build -o "$STATE/hire-server" ./cmd/server
  PORT=$API_PORT FRONTEND_ORIGIN="http://localhost:$WEB_PORT" \
    JWT_SECRET=verify-insecure-secret-0123456789abcdef \
    nohup "$STATE/hire-server" </dev/null >"$STATE/server.log" 2>&1 &
  echo $! >"$STATE/server.pid"
  wait_for "http://localhost:$API_PORT/health" 60 '"database":"up"' || { tail -20 "$STATE/server.log" >&2; exit 1; }
}

start_web() {
  log "installing web deps and starting the dev server on :$WEB_PORT"
  if ! { (cd design-system && pnpm install --frozen-lockfile) && (cd web && pnpm install --frozen-lockfile); } \
    >"$STATE/web-install.log" 2>&1; then
    tail -20 "$STATE/web-install.log" >&2; exit 1
  fi
  cd web
  VITE_API_URL="http://localhost:$API_PORT" nohup pnpm exec vite dev --port "$WEB_PORT" --strictPort \
    </dev/null >"$STATE/web.log" 2>&1 &
  echo $! >"$STATE/web.pid"
  cd "$ROOT"
  # Ready = the first SSR of /jobs answered (vite compiles on first request, ~10-30s).
  local code slug
  for _ in $(seq 1 120); do
    code=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$WEB_PORT/jobs" || true)
    if [ "$code" = 200 ]; then
      # Vite compiles each route on its first request (5-15s). Warm the job page too, so a
      # journey's first click on a posting does not time out on the compile.
      slug=$(curl -fsS "http://localhost:$API_PORT/api/v1/jobs/search?limit=1" 2>/dev/null | sed -n 's/.*"public_slug":"\([^"]*\)".*/\1/p' | head -1)
      [ -z "$slug" ] || curl -s -o /dev/null --max-time 120 "http://localhost:$WEB_PORT/jobs/$slug" || true
      return 0
    fi
    sleep 1
  done
  log "web /jobs never answered 200 (last: $code)"; tail -20 "$STATE/web.log" >&2; exit 1
}

doctor() {
  local ok=0 n
  check() { if eval "$2" >/dev/null 2>&1; then printf 'ok    %s\n' "$1"; else printf 'FAIL  %s\n' "$1"; ok=1; fi; }
  printf 'state %s  api :%s  web :%s  meili :%s  db %s\n' "$STATE" "$API_PORT" "$WEB_PORT" "$MEILI_PORT" "$DB_NAME"
  printf 'head  %s (server built from: %s)\n' "$(git rev-parse --short HEAD)" "$(cat "$STATE/built-from" 2>/dev/null || echo '?')"
  check "built from the current HEAD" '[ "$(cat "$STATE/built-from" 2>/dev/null)" = "$(git rev-parse --short HEAD)" ]'
  check "api process is ours and alive" 'alive server'
  check "api /health says database up" 'curl -fsS http://localhost:$API_PORT/health | grep -q "\"database\":\"up\""'
  check "meilisearch healthy" 'curl -fsS $MEILI_URL/health | grep -q available'
  n=$(curl -fsS "http://localhost:$API_PORT/api/v1/jobs/search?limit=1" 2>/dev/null | sed -n 's/.*"total":\([0-9]*\).*/\1/p')
  check "search has seeded postings (total=${n:-none})" '[ "${n:-0}" -gt 0 ]'
  check "web process is ours and alive" 'alive web'
  check "web /jobs answers 200" '[ "$(curl -s -o /dev/null -w %{http_code} http://localhost:$WEB_PORT/jobs)" = 200 ]'
  return $ok
}

down() {
  local p
  for p in web server meili; do
    if alive "$p"; then
      killtree "$(cat "$STATE/$p.pid")"
    fi
    rm -f "$STATE/$p.pid"
  done
  command -v docker >/dev/null 2>&1 && docker rm -f "$MEILI_CONTAINER" >/dev/null 2>&1 || true
  psql "$ADMIN_URL" -qc "DROP DATABASE IF EXISTS $DB_NAME WITH (FORCE)" >/dev/null 2>&1 || true
  rm -rf "$STATE"
  log "stopped; evidence kept in $EVIDENCE"
}

case "${1:-}" in
  up)
    if alive server || alive web; then log "an instance is already running in $STATE; run 'stack.sh down' first"; exit 1; fi
    mkdir -p "$STATE" "$EVIDENCE"
    MEILI_KEY=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
    echo "$MEILI_KEY" >"$STATE/meili-key"
    export MEILI_MASTER_KEY=$MEILI_KEY
    start_db
    start_meili
    seed
    start_api
    git rev-parse --short HEAD >"$STATE/built-from"
    start_web
    doctor
    print_env
    ;;
  doctor) doctor ;;
  env) print_env ;;
  down) down ;;
  *) sed -n '4,14p' "$SELF"; exit 2 ;;
esac
