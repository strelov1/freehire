#!/bin/bash
# Daily contribution onboarding: drain link_contributions into a pull request.
# Runs as a systemd oneshot off freehire-onboard-contributions.timer, like every
# other freehire worker.
#
# The "contribute a board" flow records a pending row per contributed board and
# never crawls anything — migration 0025 calls the status "the seam for the
# deferred ingest worker". This script is that worker: it validates each
# contributed board live, adds the ones that answer to sources/<source>.yml, and
# opens a pull request. A human merges it.
#
# Environment (systemd supplies both files; a hand run reads them itself):
#   /opt/freehire/.env                        DATABASE_URL, TELEGRAM_BOT_TOKEN
#   /opt/freehire/env/onboard-contributions.env
#       GH_TOKEN                  required, opens the pull request
#       ANTHROPIC_API_KEY         required unless CLAUDE_CODE_OAUTH_TOKEN is set
#       ANTHROPIC_BASE_URL        optional, when Claude Code runs through a proxy
#       TELEGRAM_CHAT_ID          optional, reports the run
#       REPO                      default strelov1/freehire
#       WORK_DIR                  default /var/tmp/onboard-contributions
set -uo pipefail

for env_file in "${FREEHIRE_ENV:-/opt/freehire/.env}" \
                "${ENV_FILE:-/opt/freehire/env/onboard-contributions.env}"; do
  if [ -f "$env_file" ]; then
    # shellcheck disable=SC1090
    set -a && . "$env_file" && set +a
  fi
done

REPO="${REPO:-strelov1/freehire}"
WORK_DIR="${WORK_DIR:-/var/tmp/onboard-contributions}"
# Pinned on purpose: Claude Code's own default is not among the models the proxy
# in ANTHROPIC_BASE_URL serves, and it fails the run at the first prompt.
CLAUDE_MODEL="${CLAUDE_MODEL:-claude-sonnet-4-6}"
REPO_DIR="$WORK_DIR/repo"
# Minute-resolution: a failed run leaves its branch pushed, and the next run must
# not collide with it. The open-PR guard, not the branch name, prevents pile-up.
STAMP="$(date -u +%Y%m%d-%H%M)"
LABEL="onboard-contributions"

log() { echo "$(date -u '+%Y-%m-%d %H:%M:%S') $*"; }

# Reports to Telegram when configured, and is a no-op otherwise, so the script
# is runnable by hand without a bot.
notify() {
  [ -n "${TELEGRAM_BOT_TOKEN:-}" ] && [ -n "${TELEGRAM_CHAT_ID:-}" ] || return 0
  curl -sS -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${TELEGRAM_CHAT_ID}" \
    --data-urlencode "text=$1" >/dev/null || log "WARN: Telegram notification failed"
}

die() {
  log "ERROR: $1"
  notify "Contribution onboarding FAILED: $1"
  exit 1
}

# Same connection every other freehire worker uses.
psql_hire() { psql "$DATABASE_URL" -tAF'|' -c "$1"; }

# --- preflight ---------------------------------------------------------------

for bin in claude git gh go curl psql; do
  command -v "$bin" >/dev/null 2>&1 || die "$bin not found on PATH"
done
[ -n "${DATABASE_URL:-}" ] || die "DATABASE_URL is not set"
[ -n "${GH_TOKEN:-}" ] || die "GH_TOKEN is not set"
[ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}${ANTHROPIC_API_KEY:-}" ] || die "no Claude credential is set"
export GH_TOKEN

rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR" || die "cannot create $WORK_DIR"
cd "$WORK_DIR" || die "cannot enter $WORK_DIR"

log "Cloning $REPO"
git clone --depth 50 "https://x-access-token:${GH_TOKEN}@github.com/${REPO}.git" repo >/dev/null 2>&1 \
  || die "clone failed"

# The service account has no git identity of its own, and commits are signed off,
# so set one on the clone rather than globally on the host.
git -C repo config user.name "${GIT_USER_NAME:-freehire-agent}"
git -C repo config user.email "${GIT_USER_EMAIL:-strelov1@gmail.com}"

# --- reconcile ---------------------------------------------------------------
# A board counts as onboarded once it is in main, not when its PR opens. This is
# the only write this script makes, and it only confirms what the repository
# already says.

psql_hire "SELECT id, source, board FROM link_contributions WHERE status='pending' AND source IS NOT NULL;" \
  > "$WORK_DIR/pending.tsv" || die "cannot read the queue"

: > "$WORK_DIR/merged_ids.txt"
while IFS='|' read -r id source board; do
  [ -n "${board:-}" ] || continue
  [ -f "$REPO_DIR/sources/${source}.yml" ] || continue
  if grep -qF "$board" "$REPO_DIR/sources/${source}.yml"; then
    echo "$id" >> "$WORK_DIR/merged_ids.txt"
  fi
done < "$WORK_DIR/pending.tsv"

if [ -s "$WORK_DIR/merged_ids.txt" ]; then
  ids=$(paste -sd, "$WORK_DIR/merged_ids.txt")
  psql_hire "UPDATE link_contributions SET status='onboarded' WHERE id IN (${ids});" >/dev/null \
    || die "reconcile update failed"
  log "Reconciled to onboarded: $ids"
else
  log "Nothing to reconcile"
fi

# --- guards ------------------------------------------------------------------

gh label create "$LABEL" --repo "$REPO" --color 0e8a16 \
  --description "Boards onboarded from the contribution queue" >/dev/null 2>&1 || true

open_pr=$(gh pr list --repo "$REPO" --state open --label "$LABEL" --json url --jq '.[0].url // ""' 2>/dev/null)
if [ -n "$open_pr" ]; then
  # Without this guard a week of runs leaves seven branches editing one file.
  log "An earlier pull request is still open: $open_pr"
  notify "Contribution onboarding skipped: $open_pr is still open"
  exit 0
fi

psql_hire "SELECT min(id), source, board, min(url), count(*) FROM link_contributions WHERE status='pending' AND source IS NOT NULL GROUP BY source, board ORDER BY min(created_at);" \
  > "$WORK_DIR/queue_pending.tsv" || die "cannot read pending rows"
psql_hire "SELECT id, url FROM link_contributions WHERE status='review' ORDER BY created_at;" \
  > "$WORK_DIR/queue_review.tsv" || die "cannot read review rows"

# The rejected pile is a lead list, not a graveyard, but it only changes when the
# recognizer changes — re-triaging it daily buys nothing. Look at it only when
# there is no fresh work, 20 rows at a time.
: > "$WORK_DIR/queue_rejected.tsv"
if [ ! -s "$WORK_DIR/queue_pending.tsv" ] && [ ! -s "$WORK_DIR/queue_review.tsv" ]; then
  psql_hire "SELECT id, source, board, url FROM link_contributions WHERE status='rejected' ORDER BY created_at LIMIT 20;" \
    > "$WORK_DIR/queue_rejected.tsv"
  total=$(psql_hire "SELECT count(*) FROM link_contributions WHERE status='rejected';")
  log "Re-triaging 20 of ${total} rejected rows"
fi

if [ ! -s "$WORK_DIR/queue_pending.tsv" ] && [ ! -s "$WORK_DIR/queue_review.tsv" ] && [ ! -s "$WORK_DIR/queue_rejected.tsv" ]; then
  log "Queue is empty"
  notify "Contribution onboarding: queue is empty, nothing to do"
  exit 0
fi

log "Queue: $(wc -l < "$WORK_DIR/queue_pending.tsv") pending, $(wc -l < "$WORK_DIR/queue_review.tsv") review"

# --- onboard the boards ------------------------------------------------------

cd "$REPO_DIR" || die "cannot enter $REPO_DIR"
git checkout -b "feat/onboard-contributed-boards-$STAMP" >/dev/null 2>&1 || die "cannot create the board branch"

# Claude has no web tools here, only Bash/Read/Edit/Write, so every board is
# validated with curl.
claude -p --model "$CLAUDE_MODEL" --permission-mode acceptEdits --allowedTools Bash,Read,Edit,Write -- "$(cat <<PROMPT
You are draining the crowdsourced board contribution queue. The repository is checked out in
your current working directory, on a fresh branch off main.

Input files, pipe-separated:
- $WORK_DIR/queue_pending.tsv — recognized boards: id|source|board|url|count
- $WORK_DIR/queue_review.tsv — unrecognized links: id|url
- $WORK_DIR/queue_rejected.tsv — earlier rejections worth a second look: id|source|board|url

For every row of queue_pending.tsv:

1. Already onboarded? \`grep -F "<board>" sources/<source>.yml\`. If it is there, skip the
   edit and record it as already present.
2. Validate the board live with curl. Recipes by source:
   - greenhouse: GET https://job-boards.greenhouse.io/<board> → 200 with jobs
   - lever: GET https://api.lever.co/v0/postings/<board>?mode=json → non-empty array
   - ashby: POST https://api.ashbyhq.com/posting-api/job-board/<board> → jobs
   - recruitee and other subdomain ATS: GET https://<board> → 200 careers page
   - zohorecruit: GET https://<board>/jobs/Careers → 200
   - inhire: GET https://api.inhire.app/job-posts/public/pages with header X-Tenant: <board>
   - workday (<host>/<site>): GET https://<host>/wday/cxs/<tenant>/<site>/jobs → 200
   - anything else: read internal/sources/<source>.go, find the public listing URL and
     required headers, and probe that.
   0 jobs, 404 or a hard block means the board is dead: record it as rejected and do not add
   it. When you cannot tell whether a board is live, reject it — a phantom board burns crawl
   budget every cycle, and a real board can be contributed again.
3. Resolve the company display name, cheapest signal first: og:site_name, then <title>, then
   a name field in the adapter's JSON, then a humanized slug.
4. Append it to sources/<source>.yml, following that file's own ordering and quoting. Most
   files are flat append-only lists; zohorecruit.yml is sorted alphabetically by board, so
   insert in position rather than at the end.

       - company: <Display Name>
         board: <board copied byte for byte from the queue>

Copy \`board\` verbatim — never re-derive, re-case or trim it. Ingest dedups with
\`external_id LIKE '<board>:%'\`, so any change silently doubles the board, and Workday site
case is significant.

Rules:
- One bad board must not stop the run: note it and move to the next row.
- Do not reformat unrelated entries and do not touch anything outside sources/.
- Never commit or push. The script does that.

Then write $WORK_DIR/pr_body.md — the pull request description:
- a table of onboarded boards: source, board, company, how you validated it
- the boards you turned down, with the reason
- rows from queue_review.tsv you could not resolve, marked as needing a human
- the SQL for whoever merges, so the queue matches the repository:
  UPDATE link_contributions SET status='onboarded' WHERE id IN (...);
  UPDATE link_contributions SET status='rejected' WHERE id IN (...);

Finish with one line: onboarded=<n> rejected=<n> skipped=<n>
PROMPT
)" || die "the onboarding agent failed"

if git diff --quiet -- sources; then
  log "No board passed validation"
  notify "Contribution onboarding: no contributed board passed validation, no pull request opened"
  exit 0
fi

go build ./... || die "go build failed after the board edits"
git add sources
git commit -s -m "feat(sources): onboard contributed boards" >/dev/null || die "commit failed"
branch=$(git rev-parse --abbrev-ref HEAD)
git push origin HEAD >/dev/null 2>&1 || die "push failed"

# --head is explicit because the shallow single-branch clone has no tracking ref
# for a new branch, and gh refuses to guess one.
pr_url=$(gh pr create --repo "$REPO" --head "$branch" \
  --title "feat(sources): onboard contributed boards" \
  --body-file "$WORK_DIR/pr_body.md" \
  --label "$LABEL")
# The run's deliverable is a pull request. Editing sources and failing to open
# one must fail loudly, not report success.
[ -n "$pr_url" ] || die "gh pr create returned no URL"
log "Opened $pr_url"

# --- at most one new adapter -------------------------------------------------
# Boards and adapters go to separate pull requests: a two-line YAML list is
# reviewed in a minute, a new Go adapter is code with tests, and one diff holding
# both blocks the cheap half behind the expensive one.

adapter_pr_url=""
if [ -s "$WORK_DIR/queue_review.tsv" ]; then
  git checkout -- . >/dev/null 2>&1
  git checkout main >/dev/null 2>&1 || die "cannot switch back to main"
  git checkout -b "feat/onboard-new-adapter-$STAMP" >/dev/null 2>&1 || die "cannot create the adapter branch"

  claude -p --continue --model "$CLAUDE_MODEL" --permission-mode acceptEdits --allowedTools Bash,Read,Edit,Write -- "$(cat <<PROMPT
The review queue holds links the recognizer could not map to a supported ATS. Most of them are
a company's own careers page fronting an ATS we already crawl.

1. Before fetching anything, settle them with one catalogue query. Guess company slugs from the
   URLs in $WORK_DIR/queue_review.tsv and ask the database:

       psql "\$DATABASE_URL" -c "SELECT company_slug, source, count(*) FROM jobs WHERE company_slug IN ('acme','globex') GROUP BY 1,2 ORDER BY 1;"

   Anything already crawled needs no adapter. Note it in $WORK_DIR/adapter_notes.md.
2. From what is left, pick at most ONE row that is a real, ingestable ATS with a public listing
   endpoint, and write its adapter in internal/sources, following the closest existing adapter
   in that package. Add its board file under sources/ and a test alongside the neighbouring
   adapters' tests. Single-tenant pages, dead sites, login walls and aggregators are not
   ingestable — leave them for a human.
3. If nothing qualifies, write nothing and reply NO-ADAPTER.

When you do write an adapter, also write:
- $WORK_DIR/adapter_pr_title.txt — one line, e.g. feat(sources): add <ats> adapter
- $WORK_DIR/adapter_pr_body.md — what the ATS is, the endpoint you measured, which contribution
  row (id and URL) led here, and how you validated it

Do not commit or push; the script does.
PROMPT
)" || log "WARN: the adapter agent failed, continuing"

  if [ -f "$WORK_DIR/adapter_pr_title.txt" ]; then
    title=$(cat "$WORK_DIR/adapter_pr_title.txt")
    if go build ./... && go test ./internal/sources/...; then
      git add -A
      git commit -s -m "$title" >/dev/null
      adapter_branch=$(git rev-parse --abbrev-ref HEAD)
      git push origin HEAD >/dev/null 2>&1
      adapter_pr_url=$(gh pr create --repo "$REPO" --head "$adapter_branch" --title "$title" \
        --body-file "$WORK_DIR/adapter_pr_body.md")
      log "Opened $adapter_pr_url"
    else
      log "WARN: the new adapter does not build or test clean, no pull request opened"
    fi
  fi
fi

# --- report ------------------------------------------------------------------

summary="Contribution onboarding: $pr_url"
[ -n "$adapter_pr_url" ] && summary="$summary
adapter: $adapter_pr_url"
notify "$summary"
log "Done"
