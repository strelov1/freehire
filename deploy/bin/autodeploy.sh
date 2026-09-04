#!/usr/bin/env bash
# Deploy main to production when CI has gone green on it — driven from the HOST, on a
# timer, rather than pushed in from CI.
#
# Why the host polls instead of GitHub Actions pushing:
#
#   - A self-hosted runner is the obvious answer and is the wrong one here. The freehire
#     repository is PUBLIC with 105 forks, and a runner takes jobs from GitHub: a fork's
#     pull request naming `runs-on: self-hosted` executes that fork's code on this
#     machine. Secrets stay out of a fork's reach; the machine does not.
#   - An SSH key in GitHub secrets works (restricted with `command=` it can only trigger
#     a release), but it puts a production credential in a place this host does not
#     control, and needs the inbound path to stay open.
#
# Polling inverts both. Nothing reaches in, GitHub cannot choose what runs here, and the
# only credential that leaves the box is a fine-grained token whose entire power is
# writing a commit status — a leaked one can draw a green tick and nothing else.
#
# What it costs, stated plainly: the release is no longer visible as a workflow run. It
# is a commit status plus this script's journal, and — the part that is not optional —
# a Telegram message on every outcome. A deploy path that fails silently is the failure
# mode this project keeps meeting (the .env split that muted `remind` for weeks, the
# ingest slots that skipped 56% of runs with exit 0), so the alert is load-bearing, not
# a nicety.
#
# Installed at /opt/freehire/bin/autodeploy.sh, driven by freehire-autodeploy.timer.
# ARMED BY `AUTODEPLOY=1` in /opt/freehire/.env — absent, it logs what it would have
# done and exits 0, so the timer can be installed and watched before it is trusted.
set -uo pipefail

REPO=strelov1/freehire
BRANCH=main
APP=freehire
RELEASE=/opt/freehire/bin/release.sh
# The symlink release.sh repoints to the color it just flipped to, so its HEAD is the
# commit production is SERVING — the right thing to compare against. Reading either
# color directly would compare against whichever one happens to be warm.
CURRENT=/opt/freehire/src/hire-current
ENV_FILE=/opt/freehire/.env
# Survives a reboot on purpose: it remembers which commit already failed, and a machine
# that reboots mid-incident must not come back and retry the same doomed build.
STATE_FILE=/var/lib/freehire/autodeploy.state
# A release that fails twice is not going to succeed on the third try — the build is
# broken, or the host is. Stop, keep alerting once, and let a person look. A new commit
# on main clears the count, because it is a different build.
MAX_ATTEMPTS=2
# release.sh's "another release is already running" code. Not a failure: a person is
# mid-deploy, so skip this tick silently and pick it up on the next one.
BUSY_EXIT=3

log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*"; }

# The env file is deployment state, not a checked-in script — same read as site-alert.sh.
env_get() { grep -E "^$1=" "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2-; }

armed=$(env_get AUTODEPLOY)
gh_token=$(env_get GITHUB_STATUS_TOKEN)
tg_token=$(env_get TELEGRAM_BOT_TOKEN)
tg_chat=$(env_get SITE_ALERT_CHAT_ID)

send() {
	local text=$1
	if [ -z "$tg_token" ] || [ -z "$tg_chat" ]; then
		log "TELEGRAM (not sent, channel unset): $text"
		return
	fi
	if curl -sS -m 10 -o /dev/null \
		--data-urlencode "chat_id=${tg_chat}" \
		--data-urlencode "text=${text}" \
		"https://api.telegram.org/bot${tg_token}/sendMessage"; then
		log "telegram sent: $text"
	else
		log "telegram send FAILED: $text"
	fi
}

# A commit status is the whole reason the token exists. Failing to write one must never
# fail the deploy — the release either happened or it did not, and a tick in a web UI
# does not change which.
status() {
	local sha=$1 state=$2 desc=$3
	[ -n "$gh_token" ] || return 0
	curl -sS -m 15 -o /dev/null -X POST \
		-H "Authorization: Bearer ${gh_token}" \
		-H "Accept: application/vnd.github+json" \
		-H "X-GitHub-Api-Version: 2022-11-28" \
		"https://api.github.com/repos/${REPO}/statuses/${sha}" \
		-d "$(jq -nc --arg s "$state" --arg d "$desc" \
			'{state:$s, context:"deploy/prod", description:$d}')" ||
		log "status write FAILED (${state}) — continuing, the release outcome stands"
}

# EVERY git read here runs as freehire, the user that owns the checkouts — not as root,
# which is what this script runs as.
#
# Root reading a repository owned by someone else trips git's dubious-ownership refusal
# and exits 128. It does not look like that from an ssh session, because an interactive
# root shell has HOME set and picks up /root/.gitconfig, where `safe.directory` entries
# have accumulated; a systemd unit has no such HOME, so the same command that worked by
# hand failed on the first timer tick. Setting Environment=HOME=/root in the unit would
# paper over it by depending on host state that lives in no repository. Reading as the
# owner needs nothing, and matches how release.sh already drives every git step.
deployed=$(sudo -u freehire git -C "$CURRENT" rev-parse HEAD 2>/dev/null) || {
	log "cannot read the deployed commit from ${CURRENT}; refusing to guess"
	exit 1
}
remote=$(sudo -u freehire git -C "$CURRENT" ls-remote origin "refs/heads/${BRANCH}" 2>/dev/null | cut -f1)
if [ -z "$remote" ]; then
	# ls-remote downloads nothing and is the request GitHub answers even when it is
	# throttling this address for packs, so a failure here is the network or the key —
	# not the throttle that moved this remote to SSH in the first place.
	log "cannot reach origin/${BRANCH}; skipping this tick"
	exit 0
fi

if [ "$remote" = "$deployed" ]; then
	log "up to date at ${deployed:0:9}"
	exit 0
fi

read -r state_sha state_attempts _ < <(cat "$STATE_FILE" 2>/dev/null || echo "none 0")
attempts=0
[ "$state_sha" = "$remote" ] && attempts=$state_attempts
if [ "$attempts" -ge "$MAX_ATTEMPTS" ]; then
	log "${remote:0:9} already failed ${attempts}x; not retrying until main moves"
	exit 0
fi

# Gate on CI, not on the merge. A squash merge pushes to main and the workflows start
# after it, so deploying on "main moved" would ship a commit whose tests are still
# running. Anything unfinished simply waits for the next tick — which is also what makes
# the 10-minute cadence comfortable: the wait is free.
#
# An ARRAY, not `${gh_token:+-H "..."}`: unquoted, that expansion word-splits on the
# spaces inside it and hands curl `-H`, `"Authorization:`, `Bearer`, `TOKEN"` as four
# arguments. shellcheck does not flag it. Reading the endpoint unauthenticated works for
# a public repository but shares the 60/hour-per-address anonymous budget, and this
# address is already the one GitHub throttles — so the token is used when present and
# its absence degrades rather than fails.
auth=()
[ -n "$gh_token" ] && auth=(-H "Authorization: Bearer ${gh_token}")
# The question is what THIS REPO'S OWN CI said about THIS COMMIT, which is
# `actions/runs?head_sha=...&event=push` — the workflows that ran because the commit was
# pushed to main.
#
# It used to be `commits/{sha}/check-runs`, which is a different question: everything
# GitHub has ever attached to that SHA, whoever attached it and for whatever reason. On
# 2026-09-04 that included a check run named "Dependabot", from the scheduled
# `Dependabot Updates` workflow (event=dynamic), reporting `security_update_not_possible`
# for fflate — Dependabot saying it could not raise a PR for a manifest, not a verdict on
# the commit. CI, CodeQL and gitleaks were all green, and the deploy was refused anyway.
#
# That is not a one-off: the workflow runs on its own schedule and re-attaches the same
# failure, so the fleet stops deploying ANYTHING until the underlying advisory is fixed —
# silently, at exit 0, so the timer looks healthy while production falls behind.
#
# Filtering by event rather than by name keeps the property the name list was avoiding:
# it does not drift when CI gains a job, because "ran because this commit was pushed" is
# a property of the trigger, not of a list someone maintains.
checks=$(curl -sS -m 20 \
	"${auth[@]}" \
	-H "Accept: application/vnd.github+json" \
	-H "X-GitHub-Api-Version: 2022-11-28" \
	"https://api.github.com/repos/${REPO}/actions/runs?head_sha=${remote}&event=push&per_page=100")
total=$(jq -r '.total_count // empty' <<<"$checks" 2>/dev/null)
if [ -z "$total" ]; then
	log "cannot read workflow runs for ${remote:0:9}; skipping this tick"
	exit 0
fi
if [ "$total" -eq 0 ]; then
	log "${remote:0:9} has no workflow runs yet; waiting"
	exit 0
fi
# The one race this cannot see: workflow runs that GitHub has not CREATED yet. "All of them
# passed" is only as complete as the list, so a tick landing in the seconds between a
# push and its workflows appearing would read a short list as a full one. It stays a
# note rather than a guard because the window is a few seconds against a 10-minute
# cadence, and a run that exists at all starts `queued`, which the pending check below
# already waits on. If it ever does bite, the fix is a minimum commit age, not a longer
# list of required job names — that list would drift the moment CI gains a job.
pending=$(jq -r '[.workflow_runs[] | select(.status != "completed")] | length' <<<"$checks")
if [ "$pending" -gt 0 ]; then
	log "${remote:0:9} still has ${pending} workflow(s) running; waiting"
	exit 0
fi
# `neutral` and `skipped` are passes: the watchdog jobs skip themselves on a merge, and
# a skipped check is a check that decided it had nothing to say.
failed=$(jq -r '[.workflow_runs[] | select(.conclusion | IN("success","neutral","skipped") | not) | .name] | join(", ")' <<<"$checks")
if [ -n "$failed" ]; then
	# No Telegram here on purpose. GitHub already tells whoever merged that main is red,
	# and this script would repeat it every ten minutes until someone pushed a fix.
	log "${remote:0:9} is red (${failed}); not deploying"
	exit 0
fi

if [ "$armed" != "1" ]; then
	log "WOULD DEPLOY ${deployed:0:9} -> ${remote:0:9} (AUTODEPLOY unset; set AUTODEPLOY=1 in ${ENV_FILE} to arm)"
	exit 0
fi

log "deploying ${deployed:0:9} -> ${remote:0:9}"
status "$remote" pending "release in progress"
out=$(mktemp)
"$RELEASE" "$APP" >"$out" 2>&1
rc=$?

if [ "$rc" -eq "$BUSY_EXIT" ]; then
	log "a release is already running; skipping this tick"
	status "$remote" pending "waiting for a release already in progress"
	rm -f "$out"
	exit 0
fi

# Read after the release, so the commit is in the checkout by then — and as freehire,
# for the ownership reason above. Falls back to the sha when it is not there, which is
# what happens on the failure path: release.sh may not have got as far as the pull.
subject=$(sudo -u freehire git -C "$CURRENT" log -1 --format=%s "$remote" 2>/dev/null || echo "$remote")
if [ "$rc" -eq 0 ]; then
	printf '%s 0\n' "$remote" >"$STATE_FILE"
	log "released ${remote:0:9}"
	status "$remote" success "deployed to production"
	send "✅ freehire deployed ${remote:0:9} — ${subject}"
else
	printf '%s %s\n' "$remote" "$((attempts + 1))" >"$STATE_FILE"
	log "release FAILED (exit ${rc}) for ${remote:0:9}"
	status "$remote" failure "release failed (exit ${rc})"
	# The tail is what a person needs to decide whether to re-run or to revert, and it
	# is the difference between an alert and a notification that something happened.
	send "🔴 freehire deploy FAILED (exit ${rc}) for ${remote:0:9} — ${subject}
attempt $((attempts + 1)) of ${MAX_ATTEMPTS}; journalctl -u freehire-autodeploy

$(tail -n 15 "$out")"
fi
rm -f "$out"
exit "$rc"
