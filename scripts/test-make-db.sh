#!/bin/sh
# Check command routing without connecting to any database or starting the server.
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/go" <<'STUB'
#!/bin/sh
printf '%s|%s\n' "$*" "$DATABASE_URL"
STUB
cat > "$tmp/psql" <<'STUB'
#!/bin/sh
if [ "$#" -eq 4 ] && [ "$1" = -X ] && [ "$2" = "--dbname=$DATABASE_URL" ] &&
  [ "$3" = --set=ON_ERROR_STOP=1 ] && [ "$4" = --quiet ]; then
  cat >/dev/null
  exit "${HISTORY_STATUS:-0}"
fi
[ "$#" -eq 1 ] || exit 1
printf '%s\n' "$1"
STUB
chmod +x "$tmp/go" "$tmp/psql"
if "$tmp/psql" first second >"$tmp/output" 2>&1; then
  echo 'psql stub must reject extra arguments' >&2
  exit 1
fi
export PATH="$tmp:$PATH"
export DATABASE_URL='postgres://localhost/checkout_test?sslmode=disable&application_name=make test'
[ "$(make -s run)" = "run ./cmd/server|$DATABASE_URL" ]
[ "$(make -s migrate)" = "run ./cmd/migrate|$DATABASE_URL" ]
if HISTORY_STATUS=1 make -s migrate >"$tmp/output" 2>&1; then
  echo 'migrate must refuse a failed migration-history check' >&2
  exit 1
fi
if rg -q 'run ./cmd/migrate' "$tmp/output"; then
  echo 'migrate must not run after a failed migration-history check' >&2
  exit 1
fi
[ "$(make -s psql)" = "$DATABASE_URL" ]
if (unset DATABASE_URL; make -s psql) >"$tmp/output" 2>&1; then
  echo 'psql must refuse an unset DATABASE_URL' >&2
  exit 1
fi
echo 'Make database routing passed'
