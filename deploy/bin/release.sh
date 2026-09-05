#!/usr/bin/env bash
# Blue/green release for freehire. Usage: release.sh [freehire]
# Builds the INACTIVE color from its own checkout, health-checks, flips nginx,
# rebuilds the worker binaries, and repoints the `hire-current` symlink (workers
# follow the active release). Old color stays warm for rollback.
set -euo pipefail
# One release at a time, enforced here rather than by convention.
#
# Until autodeploy.sh existed, "a person is running it" WAS the lock: nothing on this
# host holds a file lock, and systemd's refusal to start a second instance of a oneshot
# unit only ever protected the timer path. A timer that releases on its own introduces
# the collision that could not happen before — a scheduled run starting while an
# operator is mid-flip — and two releases sharing one inactive color would build over
# each other's checkout and flip nginx at whichever finished first.
#
# `flock -n` so this fails immediately with something readable instead of a script that
# appears to hang. Exit 3 is a distinct code on purpose: autodeploy.sh reads it as "not
# my turn, try the next tick" rather than as a failed release, so a hand-run deploy does
# not raise an alert. /var/lock is tmpfs, so a reboot mid-release cannot leave it held.
exec 9>/var/lock/freehire-release.lock
if ! flock -n 9; then
  echo "release: another release is already running; refusing to start a second." >&2
  exit 3
fi
app="${1:-freehire}"
case "$app" in
  freehire) snip=freehire-upstream; apisvc=freehire-api; websvc=freehire-web; dir=hire; bin=hire-api; b_api=8081; b_web=8083; g_api=8082; g_web=8084 ;;
  *) echo "usage: release.sh [freehire]" >&2; exit 1 ;;
esac
S=/etc/nginx/snippets
active=$(readlink -f "$S/${snip}-active.conf")
case "$active" in
  *-blue.conf)  new=green; aport=$g_api; wport=$g_web ;;
  *-green.conf) new=blue;  aport=$b_api; wport=$b_web ;;
  *) echo "release: cannot determine active color from $active" >&2; exit 1 ;;
esac
echo "[release:$app] target inactive color: $new (api:$aport web:$wport)"
# The flip at the end of this script is a symlink swap, and nginx serves whatever
# upstream the file behind that symlink defines. So a color whose conf names the OTHER
# color's ports turns the whole release into a no-op: it builds, health-checks, flips,
# prints Done, and keeps serving the old code.
#
# Not hypothetical. freehire-upstream-green.conf was overwritten with blue's ports
# (8081/8083) on 2026-08-20 and stayed that way, so every release that targeted green
# was silently discarded — and since this script always targets the INACTIVE color,
# that was every OTHER release. Nothing downstream catches it: the health check curls
# 127.0.0.1:$aport directly, which passes fine on a color nginx will never route to.
#
# Assert it here rather than after the build: five minutes of compiling for a flip that
# cannot take traffic is the expensive way to find out. The canonical files live in
# freehire-ops (provision/host2/nginx/${snip}-{blue,green}.conf) — the server had drifted
# from them, which is the only reason this check has anything to say.
newconf="$S/${snip}-${new}.conf"
if ! grep -q "127\.0\.0\.1:${aport};" "$newconf" || ! grep -q "127\.0\.0\.1:${wport};" "$newconf"; then
  echo "release: $newconf does not point at $new's own ports (api:$aport web:$wport)." >&2
  echo "release: flipping to it would keep serving the other color. Refusing to build." >&2
  echo "release: expected contents are in freehire-ops provision/host2/nginx/${snip}-${new}.conf" >&2
  sed 's/^/release:   /' "$newconf" >&2
  exit 1
fi
cd "/opt/freehire/src/${dir}-${new}"
# protocol.version=0, and the reason is worth the paragraph because the error message
# points at the wrong thing entirely. On 2026-09-02 every release began dying with:
#
#   fatal: could not read Username for 'https://github.com': No such device or address
#
# The repository is public, `curl` as the same user fetched the same URL with a 200, and
# there is no credential anywhere on the box — yet git was asking to log in. Tracing the
# exchange showed why. Git's protocol v2 makes TWO requests, and GitHub answers them
# differently for this host:
#
#   GET  /info/refs?service=git-upload-pack  -> 200
#   POST /git-upload-pack                    -> 401  www-authenticate: Basic realm="GitHub"
#
# Protocol v0 needs only the first and succeeds: measured 10/10 where v2 managed 2/10.
# The 2-in-10 is what made this look intermittent and cost an hour — it is not a flaky
# network, it is two code paths, and retrying only repeats the doomed request. "No such
# device or address" is a MISSING TERMINAL, not a refused credential; a scripted release
# has no terminal to prompt on, so the first 401 ends it.
#
# Why GitHub singles out the v2 POST from here is not established — this host crawls
# hard, so a throttle on its address is the likeliest reason.
#
# The pin bought about three hours. Later the same day the SAME 401 came back on v0,
# which is the answer to what the paragraph above left open: the throttle is not on a
# protocol version, it is on ANONYMOUS object fetches from this address. Both versions
# POST to /git-upload-pack to download a pack; v0 merely reaches it second. What still
# succeeded, and is the tell, was every request that downloads nothing:
#
#   git ls-remote  (GET  /info/refs)   -> 200, repeatedly
#   git fetch      (POST /git-upload-pack) -> 401, 3/3
#
# So the durable fix the paragraph above named is the one now in place: the remote is
# authenticated. `origin` is git@github.com:strelov1/freehire.git in BOTH colors, and
# ~freehire/.ssh/config points github.com at the agent_deploy key already on the box
# (provisioned 2026-07-14). No new secret was introduced — that key is issued to
# strelov1/freehire-agent and reads this repository only because this repository is
# public, which is worth replacing with a deploy key of its own when convenient.
#
# The protocol pin stays. It costs nothing, it is orthogonal to the transport, and
# leaving it removes one variable if the fetch path is ever in question again.
# GIT_TERMINAL_PROMPT=0 likewise stays: it keeps any future variant failing fast
# instead of hanging on a prompt nobody can answer.
#
# If this fails again with a credential error, check the KEY first (`sudo -u freehire
# ssh -T git@github.com` names the identity it authenticated as), not the protocol.
sudo -u freehire env GIT_TERMINAL_PROMPT=0 git -c protocol.version=0 pull --ff-only
echo "[release:$app] building api + web ..."
sudo -u freehire /usr/local/bin/go build -buildvcs=false -o "$bin" ./cmd/server
# The design system is a `link:` dependency (freehire#1335) — a bare symlink into
# ../design-system, so there is no copy to go stale. It was `file:` until 2026-07-31, which
# COPIES the package into web's virtual store keyed by name+version, and its version is a
# permanent 0.0.0: `pnpm install --frozen-lockfile` reused the copy from the previous release
# however much the package changed. That failed loudly once (freehire#1300, a build error for
# a theme.css the copy did not have) and silently once — the app shipped markup referencing
# `bg-warning` against a design system defining no such token, so every caution badge on the
# site rendered colourless, with a green release and a 200 from /health.
#
# The cost of a symlink is that pnpm does NOT install the linked package's dependencies with
# web's. tailwind-variants, clsx, tailwind-merge and @lucide/svelte resolve out of
# design-system's OWN node_modules, so it has to be installed first or the web build dies with
# MODULE_NOT_FOUND. Loud, which is the whole trade.
( cd design-system && sudo -u freehire corepack pnpm install --frozen-lockfile )
# Source-map upload credentials for the Sentry vite plugin, if provisioned. Without them the
# plugin skips the upload and the build still succeeds — Sentry then shows minified frames
# (`t.$$render` at line 1) instead of real ones, which is the only thing this buys. The maps
# stay published either way: freehire is open source, so there is nothing to withhold.
#
# The file is 0600 root and read here, not exported into the unit, so the token never reaches
# the running app. `--preserve-env` rather than `env VAR=...` keeps it out of ps(1) too.
SENTRY_BUILD_ENV=/opt/freehire/env/sentry-build.env
if [ -r "$SENTRY_BUILD_ENV" ]; then
	set -a
	# shellcheck source=/dev/null
	. "$SENTRY_BUILD_ENV"
	set +a
	echo "[release:$app] source maps will be uploaded to Sentry (${SENTRY_ORG:-?}/${SENTRY_PROJECT:-?})"
fi
# web migrated from npm to pnpm (DS Phase 2, freehire#1088): install/build via corepack,
# which provisions the pnpm version pinned in web/package.json's packageManager field.
( cd web && sudo -u freehire corepack pnpm install --frozen-lockfile &&
	sudo -u freehire --preserve-env=SENTRY_ORG,SENTRY_PROJECT,SENTRY_AUTH_TOKEN corepack pnpm run build )
# The web build can succeed-with-exit-0 and still leave no client bundle: on 2026-07-27 vite's
# closeBundle rimraf of build/client/_app raced the precompress step, deleting all 2409 assets
# and leaving only version.json.{gz,br}. Nothing below catches that — SSR renders fine without
# client assets, so the node server starts and the health-check passes, and the broken color
# took traffic. Gate on the bundle actually being there. The floor is deliberately far below a
# real build (~2400 files) so it fires only on this kind of wipe, not on bundle-size drift.
assets=$(find web/build/client/_app -type f 2>/dev/null | wc -l)
if [ "$assets" -lt 100 ]; then
  echo "release: web client build is empty ($assets files under web/build/client/_app) — refusing to release $new" >&2
  echo "release: rm -rf web/build in that checkout and re-run; the live color is untouched" >&2
  exit 1
fi
echo "[release:$app] web client bundle ok ($assets files)"
# The second guard on the same build, for CONTENT rather than count. The one above catches a
# bundle that is not there; this catches one built against a different copy of the design
# system than the checkout holds — the failure the rm above prevents, asserted rather than
# trusted, because the rm is a prevention and preventions rot. Every custom property the
# checkout's token build defines has to appear in the CSS this build emitted; the token names
# are read from the file rather than listed here, so it covers whatever ships next without
# being touched. On 2026-07-31 exactly four were missing (--warning and its three siblings)
# out of seventy-nine.
if [ -z "$(find web/build/client/_app -name '*.css' -type f 2>/dev/null | head -1)" ]; then
  echo "release: no stylesheet in the web bundle — refusing to release $new" >&2
  exit 1
fi
# `grep -r --include` rather than a list of filenames in a variable: an unquoted list only
# splits into arguments because this runs under bash, and it would stop working the day a
# path has a space in it. One directory, no splitting.
missing=$(grep -oE '^  --[a-z0-9-]+:' design-system/dist/tokens-light.css | tr -d ' :' |
  while read -r t; do
    grep -rqF --include='*.css' -- "$t:" web/build/client/_app || echo "$t"
  done)
if [ -n "$missing" ]; then
  echo "release: the web bundle is missing design tokens this checkout defines:" >&2
  # sed, not ${var//}: the substitution indents EVERY line of a multi-line list,
  # which bash's parameter expansion cannot express (^ is not per-line there).
  # shellcheck disable=SC2001
  echo "$missing" | sed 's/^/  /' >&2
  echo "release: the build did not see this checkout's design-system." >&2
  echo "release: check that design-system/node_modules exists and dist/ is built; the live color is untouched" >&2
  exit 1
fi
echo "[release:$app] web bundle carries the checkout's design tokens"
if [ "$app" = freehire ]; then
  echo "[release:$app] building worker binaries ..."
  # Reference point for the completeness check below: anything the loop builds lands after it.
  marker=$(mktemp) && trap 'rm -f "$marker"' EXIT
  # Every binary a freehire-*.service ExecStarts from hire-current must be listed here, or
  # its timer keeps firing the copy built by whichever release last happened to include it.
  # reindex-companies was missing and ran 6-day-old code off its daily timer (2026-07-27).
  # The backfill-* one-offs have no unit; they are listed so a manual repair run does not
  # need a hand build on the box. harvest-orphans is the same class: it reads the catalogue
  # for companies held only through aggregators and writes a candidate-board seed, so it has
  # to run where DATABASE_URL is (freehire#1413). import-collections is the same class too —
  # missing from this list until the us-h1b-sponsor rollout (2026-08-05) needed a hand build
  # on the box mid-incident, exactly the failure mode this comment already warns about. Built
  # here rather than by hand, because a hand-built binary silently disappears on the next
  # release. import-yc and import-company-industries joined for the same reason: both write
  # companies.industries through the curated dictionary, and a stale copy of either would
  # write a vocabulary the current code no longer agrees with.
  # onboarding, broadcast and backfill-slug-folded were found MISSING from this list on
  # 2026-08-16, while the copy actually installed at /opt/freehire/bin/release.sh carried all
  # three — the list on the box had been edited in place and the edit never came back to git.
  # That is the same failure this comment block keeps warning about, arriving from the other
  # direction: not a worker missing from prod, but a worker missing from the repo, so the next
  # person to deploy this file from git would silently stop rebuilding three binaries.
  # queue-metrics joined 2026-08-15 with the pipeline alerts. It matters more than most
  # entries here: its own staleness IS an alert (pipeline-exporter-stopped), so a missing
  # binary does not fail quietly — it pages, every ten minutes, forever.
  # nudge, apple-revoke, auth-cleanup and capture-apply-form joined 2026-08-21, found by the
  # check below on its first run. All four had enabled timers firing binaries this list had
  # never built: nudge, apple-revoke and auth-cleanup were six days old, capture-apply-form
  # sixteen. Their units were hand-installed on the box and never reached git either (#56) —
  # the same half-provisioned move, arriving here as a stale binary instead of a missing file.
  # backfill-company-type-hint joined 2026-09-04, ahead of its own first run, for the same
  # reason import-collections was added: a one-off with no unit should not need a hand build
  # on the box the first time it runs either.
  # backfill-board-catalog left 2026-09-04: freehire#2406 deleted its cmd/ entirely (sources/,
  # the board catalog it backfilled, was retired), and this list still tried to build it on
  # every release — the drift this comment keeps warning about, arriving as a directory that
  # no longer exists instead of one nothing built. No systemd unit ever referenced it (checked
  # both /etc/systemd/system and this repo's provision/ before removing), so nothing to add
  # back on the other side.
  # billing-sync and build-suggestions joined 2026-09-04, found by the check below on this
  # release: both had freehire-*.service units already installed on the box, firing whatever
  # binary was last built by hand — the same arrival this comment keeps describing, just two
  # more of it.
  # auto-apply-orchestrate joined 2026-09-05: a long-lived Inngest function server, not a
  # cron worker, but it ships from hire-current the same way every other binary here does.
  for w in migrate onboarding broadcast ingest enrich embed similar-backfill search-drain reindex reindex-companies import-collections import-yc import-company-industries queue-metrics tg-ingest tg-extract liveness notify remind nudge apple-revoke auth-cleanup capture-apply-form backfill-derive backfill-company-names backfill-descriptions backfill-application-events backfill-slug-folded backfill-duplicate-marker-owner backfill-company-type-hint billing-sync build-suggestions merge-companies add-board harvest-orphans recount-companies rollup-stats rollup-facets rollup-company rollup-views classify-mail resolve-url gmail-sync cal-sync mail-ingest hydrate-adzuna-description seed-adzuna-description-queue ingest-scheduler schedule-board auto-apply-orchestrate social-digest; do
    sudo -u freehire /usr/local/bin/go build -buildvcs=false -o "$w" "./cmd/$w"
  done
  # Every binary a freehire-*.service starts from hire-current has to have just been built,
  # or its timer keeps firing whatever the last release that happened to include it left
  # behind. The list above is hand-kept, and the comment on it has been asking people to
  # keep it in step since 2026-07-27 — asking is what failed. Derive the requirement from
  # the units instead and say so out loud.
  #
  # `-nt` against a marker stamped at the top of this loop, not an mtime read: a binary the
  # loop rebuilt is newer than the marker even when the source did not change, because `go
  # build -o` rewrites the file. Reported and not fatal — a release that stops on this would
  # be refusing to ship the API over a stale cron worker, which is the wrong trade — but
  # loud, because the whole failure mode is that nothing said anything.
  stale=$(grep -h "^ExecStart=" /etc/systemd/system/freehire-*.service 2>/dev/null |
    grep -oE "/opt/freehire/src/hire-current/[a-z0-9-]+( |$)" | tr -d " " | sed "s|.*/||" |
    sort -u |
    while read -r b; do
      [ -d "./$b" ] && continue   # a path, not a binary (ingest@ points at sources/)
      if [ ! -x "./$b" ] || [ ! "./$b" -nt "$marker" ]; then echo "$b"; fi
    done)
  if [ -n "$stale" ]; then
    echo "[release:$app] WARNING: a unit starts these from hire-current, but this release did not build them:" >&2
    # Same per-line indent as above; see the note there.
    # shellcheck disable=SC2001
    echo "$stale" | sed "s/^/[release:$app]   /" >&2
    echo "[release:$app]   their timers are firing whatever an older release left. Add them to the list in $0." >&2
  fi
  # mail-ingest is a long-lived daemon (not a timer): restart it so it picks up the
  # freshly built binary from the repointed hire-current. Enabled once via provision.
  # auto-apply-orchestrate is restarted separately, AFTER the flip below — it calls
  # hire's own API over loopback at a fixed blue/green port, so restarting it here
  # (before $new is actually active) would have it come up pointed at whichever color
  # is about to go warm-standby, not the one about to take traffic.
  systemctl try-restart freehire-mail-ingest.service 2>/dev/null || true
fi
# Schema BEFORE the code that reads it. This exists because on 2026-07-29 a release carried
# a merged migration nobody had applied: sqlc reads every column of a table, so one missing
# `jobs.ats_absent_at` returned 42703 on EVERY job read — the catalogue, the job pages, the
# search — not just the feature that added it. Nothing in the pipeline noticed, because
# /health does not touch jobs and the migration was somebody else's.
#
# The runner is idempotent (one transaction per file, recorded in schema_migrations under an
# advisory lock), so running it on every release is a no-op when there is nothing new. It runs
# BEFORE the new color starts, so that color never serves a request against an older schema.
#
# A migration failure aborts the release with the live color untouched: a half-migrated
# database serving new code is the state this whole block exists to prevent.
if [ "$app" = freehire ]; then
  echo "[release:$app] applying migrations ..."
  sudo -u freehire /usr/local/bin/go build -buildvcs=false -o migrate ./cmd/migrate
  # The env file is deployment state, not a checked-in script — see logo-cache-backup.sh.
  # shellcheck disable=SC1091
  ( set -a; . /opt/freehire/.env; set +a; sudo -u freehire -E ./migrate ) || {
    echo "[release:$app] migrations FAILED — not touching $new or the live color" >&2
    exit 1
  }
fi
systemctl restart "${apisvc}@${new}" "${websvc}@${new}"
echo "[release:$app] health-checking $new ..."
ok=
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$aport/health" >/dev/null 2>&1 && curl -fsS -o /dev/null "http://127.0.0.1:$wport/" 2>/dev/null; then ok=1; break; fi
  sleep 1
done
[ -n "$ok" ] || { echo "[release:$app] health-check FAILED on $new — NOT flipping"; exit 1; }
# Facet smoke: a new filterable/facet attribute 500s until the Meili index is
# reindexed to register it (e.g. #663's is_tech). /health can't see this — it needs
# a real facet query. Probe the bare distribution plus the disjunctive+filtered path
# the SPA hits, so shipping a facet without its reindex aborts here, not in prod.
if [ "$app" = freehire ]; then
  echo "[release:$app] facet smoke on $new ..."
  fok=
  for _ in $(seq 1 15); do
    if curl -fsS -o /dev/null "http://127.0.0.1:$aport/api/v1/jobs/facets" 2>/dev/null \
       && curl -fsS -o /dev/null "http://127.0.0.1:$aport/api/v1/jobs/facets?work_mode=remote&disjunctive=1" 2>/dev/null; then fok=1; break; fi
    sleep 1
  done
  [ -n "$fok" ] || { echo "[release:$app] facet smoke FAILED on $new — a facet attr likely needs a reindex; NOT flipping"; exit 1; }
fi
# Keep this build reachable after the next flip, for the tabs still running it.
#
# nginx serves /_app/immutable/ off hire-current (snippets/freehire-app.conf), the
# symlink the flip below repoints in one step -- so without this, every chunk of
# the outgoing build stops existing the instant traffic moves, and a tab that was
# open across the release 404s on its next navigation. SvelteKit draws that as the
# 500 page over an HTTP 200. The client-side guard in web/src/routes/+layout.svelte
# rescues the tab that has already noticed a new build; it cannot rescue the one
# navigating inside the five-minute version-poll window, and this is that half.
#
# Safe because every name under _app/immutable carries a content hash: a copy kept
# from an older build can never disagree with a live one about what it contains.
#
# That is also why this OVERWRITES rather than skipping what is already there. The
# bytes are identical either way, so the only thing rewriting them buys is a fresh
# timestamp -- which is what makes a file's age in here mean "since the last build
# that shipped this chunk" instead of "since it was first seen". Skipping would
# evict a chunk that had been stable all week on the day before the build that
# finally replaced it, leaving that hash in neither directory: precisely the 404
# this exists to prevent, arriving only for the chunks that actually changed.
#
# The prune runs whether or not the copy did, because a copy fails on a full disk
# and that is the one time reclaiming matters. Neither step is fatal: a release
# that cannot fill the attic is still a good release, and refusing to ship over it
# would trade a rare stale-tab 404 for a stuck deploy.
if [ "$app" = freehire ]; then
  attic=/opt/freehire/asset-attic/_app/immutable
  install -d -o freehire -g freehire "$attic" 2>/dev/null || true
  if cp -rf "/opt/freehire/src/hire-${new}/web/build/client/_app/immutable/." "$attic/"; then
    chown -R freehire:freehire /opt/freehire/asset-attic 2>/dev/null || true
  else
    echo "[release:$app] WARNING: could not refill the asset attic; tabs open across the NEXT flip may 404 on this build's chunks" >&2
  fi
  # `-mtime +3` is "older than three full days", so a chunk outlives by at least
  # three days the last build that shipped it.
  find "$attic" -type f -mtime +3 -delete 2>/dev/null || true
  find "$attic" -type d -empty -delete 2>/dev/null || true
  echo "[release:$app] asset attic holds $(find "$attic" -type f | wc -l) files ($(du -sh "$attic" | cut -f1))"
fi

ln -sf "$S/${snip}-${new}.conf" "$S/${snip}-active.conf"
nginx -t && nginx -s reload
if [ "$app" = freehire ]; then
  ln -sfn "/opt/freehire/src/hire-${new}" /opt/freehire/src/hire-current
  echo "[release:$app] workers now follow hire-${new} (hire-current repointed)"
  # auto-apply-orchestrate calls hire's own API over loopback at a fixed blue/green
  # port (internal/platform/config's own PORT default, 8080, matches neither — found
  # 2026-09-05 the hard way: every call failed with connection refused until this
  # existed). Regenerated on every release, after $new is the color nginx just sent
  # traffic to, so a restart now always picks up the currently-active port rather
  # than whichever color is about to go warm-standby.
  echo "HIRE_BASE_URL=http://127.0.0.1:${aport}/api/v1" > /opt/freehire/env/auto-apply-orchestrate.env
  chown freehire:freehire /opt/freehire/env/auto-apply-orchestrate.env
  systemctl try-restart freehire-auto-apply-orchestrate.service 2>/dev/null || true
fi
echo "[release:$app] flipped to $new; previous color warm for rollback"

# Cloudflare holds anonymous HTML at the edge (infra/cloudflare/), so a flip that
# changes what a page renders leaves the superseded copy being served until its TTL
# runs out. Purge AFTER the flip, never before — a purge issued while the old colour
# is still active only refills the cache from the old build.
#
# This must never fail the release. By this line the flip has already happened;
# aborting here would report failure for a release that succeeded, and the worst
# case of a missed purge is one TTL of stale HTML. Hence the explicit `set +e` — the
# script runs under `set -euo pipefail`, where an unguarded non-zero curl would exit.
#
# `purge_everything` also evicts the hashed `/_app/immutable/*` chunks, which is not
# the waste it looks like: the flip changed their hashes, so those entries are for
# URLs the new colour will never serve, and nginx refills them off disk
# (`location ^~ /_app/immutable/`), not through the Node SSR process.
#
# Only the token needs provisioning — the zone is not a secret and is the same one
# `infra/cloudflare/` describes. Missing token is a SKIP, not a failure, so a host
# that has not been given one still deploys.
CF_ZONE_DEFAULT=41b0d0bd7b22a66a6d3211079c6fe2fe
cf_env=/opt/freehire/.env
cf_token=$(grep -E '^CLOUDFLARE_PURGE_TOKEN=' "$cf_env" 2>/dev/null | head -1 | cut -d= -f2- || true)
cf_zone=$(grep -E '^CLOUDFLARE_ZONE_ID=' "$cf_env" 2>/dev/null | head -1 | cut -d= -f2- || true)
cf_zone=${cf_zone:-$CF_ZONE_DEFAULT}
if [ -z "${cf_token:-}" ]; then
  echo "[release:$app] cloudflare purge SKIPPED (no CLOUDFLARE_PURGE_TOKEN in $cf_env)"
else
  set +e
  cf_out=$(curl -sS --max-time 20 -X POST \
    "https://api.cloudflare.com/client/v4/zones/${cf_zone}/purge_cache" \
    -H "Authorization: Bearer ${cf_token}" \
    -H 'Content-Type: application/json' \
    --data '{"purge_everything":true}' 2>&1)
  cf_rc=$?
  set -e
  if [ "$cf_rc" -eq 0 ] && printf '%s' "$cf_out" | grep -q '"success":true'; then
    echo "[release:$app] cloudflare edge cache purged"
  else
    echo "[release:$app] cloudflare purge FAILED (rc=$cf_rc); release STANDS, edge may serve stale HTML for one TTL: $cf_out"
  fi
fi
