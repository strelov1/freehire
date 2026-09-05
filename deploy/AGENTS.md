# deploy

The production host's systemd units and operator scripts, as they run on host-2. Not Go,
not built, not imported by anything — this directory is a **record**, and the only reason
it exists is that the machine was the sole copy.

Snapshot taken 2026-09-05 from `/etc/systemd/system/freehire-*`, `/opt/freehire/bin/*.sh` and
`/etc/nginx/snippets/freehire-app.conf`.

## What is here

```
systemd/   337 files — 46 .service, 286 .timer, 5 drop-in directories
bin/        16 operator scripts (release, autodeploy, backups, alerting, ingest slotting)
nginx/       1 snippet — snippets/freehire-app.conf, hand-edited, not generated
```

`nginx/` holds one file on purpose. `freehire-app.conf` decides how `/_app/immutable/` is
served, which is roughly three quarters of all requests and where the asset attic below
lives; it was the last load-bearing thing on this host whose only copy was the machine.
The other `freehire-*` snippets are left out because `freehire-upstream-active.conf` is
the symlink the flip repoints, so tracking it would report drift after every release and
teach the reader to ignore the tool.

`systemd/freehire-ingest@.service` is one template; the `freehire-ingest@<provider>.timer`
files beside it are per-provider schedules, which is why the timer count dwarfs everything
else. `bin/gen-ingest-timers.sh` writes them from the board catalog
(`SELECT provider FROM boards WHERE status IN ('pending','active')`) — it is not run by
anything, so a new provider means running it on the host.

**Both are being retired.** `freehire-ingest-scheduler.timer` runs
`cmd/ingest-scheduler` once a minute, which reads the same catalog and starts each due run
as a TRANSIENT unit — so there is no per-provider file to generate and no second spelling
of a provider key to drift from `boards`. The generator's "not run by anything" above is
exactly what that fixes: between its manual runs the units were a photograph of a catalog
that had moved, and two providers died in that gap (see
[internal/ingest/ingestsched/AGENTS.md](../internal/ingest/ingestsched/AGENTS.md)).
During the cutover BOTH mechanisms are installed and each provider is driven by exactly one
of them, decided by `ingest_schedule.managed`; the generated files and
`bin/gen-ingest-timers.sh` and `bin/ingest-slot.sh` all go once every provider is managed.
**The scheduler ships in shadow mode** (`INGEST_SCHEDULER_APPLY` unset), so installing it
changes nothing until an operator turns launches on.

**`bin/autodeploy.sh` is what actually ships main to production**, on a 10-minute timer:
it waits for the commit to be green and then calls `release.sh`. What "green" means is the
one thing worth knowing about it — see the comment above its check, which records the day
a scheduled Dependabot run made every deploy stop, silently, at exit 0.

## Always true

- **Nothing here deploys itself.** `release.sh` builds and flips the app; it does not touch
  units or the scripts in this directory. Changing a file here is half the job — the other
  half is copying it to the host and running `systemctl daemon-reload`. Treat git as the
  truth and the host as the copy, not the other way round, or this snapshot rots into
  fiction within a month.
- **Billing reads four variables from `/opt/freehire/.env`, and is inert without them.**
  `STRIPE_SECRET_KEY` (`sk_…`), `STRIPE_WEBHOOK_SECRET` (`whsec_…`), `STRIPE_PRICE_IDS`, and
  `FRONTEND_ORIGIN` — which the fleet already sets, and which is reused rather than given a
  billing-specific twin that would disagree with it the first time one was changed. With any missing, every billing route is simply not mounted and the timer is a
  no-op that never opens the pool — so the units are safe to install before the provider is
  ready, they are just inert.

  **`STRIPE_PRICE_IDS` is a comma-separated list, and the FIRST is what a new subscriber is
  sold.** The rest stay recognised so that somebody on an older or annual price keeps their
  plan. An empty list is not a default that sells the obvious thing — it confers Pro on
  nobody, deliberately, because the alternative to refusing a misconfiguration is granting
  one.

  Two steps happen in the provider's dashboard and nowhere else: registering the webhook at
  `https://freehire.me/api/v1/billing/stripe/webhook` (its `whsec_` secret is shown once,
  on that screen), and creating the price. The rest is API.

  **The webhook secret differs between the dashboard endpoint and `stripe listen`.** Using
  one to verify the other fails every delivery, and the failure looks exactly like a wrong
  key rather than a wrong environment.

- **A new worker needs its binary built on the host.** `release.sh` builds the API, not
  every command in `cmd/`. `billing-sync` is the first addition since that was last true;
  build it where the other worker binaries live before enabling the timer, or the unit
  fails on a missing executable every hour.
- **The environment is split across two files, and the split is a trap.** Every unit reads
  `/opt/freehire/.env`; the mail credentials (`NOTIFY_EMAIL_FROM` plus the SES keys) live
  ONLY in `/opt/freehire/.env.notify`. A worker that sends mail and reads just the first
  loses its email channel — and does not fail, because "channel not configured" is a
  deliberate soft-skip. **The five workers that send mail are `notify`, `nudge`, `remind`,
  `broadcast` and `onboarding`**, and each must read both files. `remind` and `nudge` did
  not, from the day they shipped until 2026-09-01: 244 email reminders piled up unsent
  across 43 people while every run exited 0 with `failed=0`. Neither env file is in git and
  neither should be.
- **A `.d/` drop-in beside a unit is how the host adds to it**, and both spellings are in
  use here: `mail.conf` adds the env file above, `10-timeout.conf` and
  `10-skip-if-reindexing.conf` adjust one setting. A drop-in's directives apply after the
  unit's own, which is what makes `EnvironmentFile=` ordering come out right — the later
  file wins on a key both define (`AWS_REGION` is in both).
- **The release fetches over SSH, and the two pieces that make that work are not in git.**
  `origin` is `git@github.com:strelov1/freehire.git` in both `/opt/freehire/src/hire-blue`
  and `hire-green`, and `~freehire/.ssh/config` (the account's home is `/var/lib/freehire`,
  not `/home`) points `github.com` at the `agent_deploy` key beside it. Neither a remote URL
  nor a private key belongs in this snapshot, so `check-drift.sh` cannot see either — this
  paragraph is the record. The reason is in `bin/release.sh` beside the `pull`: GitHub
  throttles ANONYMOUS object fetches from this address, and the repository being public is
  not enough, because the throttle lands on the pack download rather than on the ref list.
  A release that dies with *"could not read Username"* is that, or the key; it is not a
  missing credential file and not the protocol version, which was the first diagnosis and
  held for three hours. The key is issued to `strelov1/freehire-agent` and reads this
  repository only because this repository is public — a deploy key of its own would be
  tidier and is not urgent.
- **The `.bak` files on the host are not here on purpose.** There are a dozen dated copies
  of `release.sh` alone. They are what a directory without version control grows instead of
  history; this directory is the replacement, so they were dropped rather than imported.
- **The binaries in `/opt/freehire/bin` are not here either** — `hire-api`, `logo-proxy`,
  `meilisearch`, `rollup-views`, `seed-from-nginx` are compiled artifacts, and a committed
  binary breaks `git pull` on the host.
- **The scripts are shellcheck-clean and CI enforces it.** The `artifacts` job runs
  shellcheck over every tracked `*.sh`, so these 16 are covered the moment they are
  committed. The findings that came with them were all style-level and are suppressed
  inline with the reason beside them — none was a defect.

## The asset attic

`/opt/freehire/asset-attic/_app/immutable/` holds the client chunks of recent builds, and
nginx falls back to it when the live build does not have the file (`location @attic` in
`nginx/snippets/freehire-app.conf`). `release.sh` copies each build in and drops what no
build has shipped for three days; about 39 MB a build.

It exists because `/_app/immutable/` is served off `hire-current`, the symlink the flip
repoints in one step — so before this, every chunk of the outgoing build stopped existing
the instant traffic moved, and a tab open across the release 404'd on its next navigation,
which SvelteKit draws as the 500 page over an HTTP 200. Keeping the files is safe by
construction: every name under `_app/immutable` carries a content hash, so a kept copy can
never disagree with a live build about what it contains.

It is the second half of a fix, not a replacement for the first.
`web/src/routes/+layout.svelte` leaves through a full page load once `updated` reports a
new build, which took these from 265 a day to about 22; what it cannot catch is a reader
navigating inside the five-minute version poll, and no client-side guard can. Neither half
is redundant: the client one also covers a stale tab older than the attic's three days.

**An empty attic protects nobody for one release.** `release.sh` banks the build it is
about to make live, so the build going *warm* was only ever banked by the release before
it. The first run after this shipped had nothing behind it, and the same hole opens after
any hand-wipe of the directory. Seeding it costs one command per colour, and both were run
on 2026-09-05 when this went in:

```bash
install -d -o freehire -g freehire /opt/freehire/asset-attic/_app/immutable
for c in blue green; do
  cp -rf "/opt/freehire/src/hire-$c/web/build/client/_app/immutable/." \
         /opt/freehire/asset-attic/_app/immutable/
done
chown -R freehire:freehire /opt/freehire/asset-attic
```

**To undo it**, restore the snippet and reload — the directory can then be deleted at
leisure, since nothing but that `location` block reads it:

```bash
ls /etc/nginx/snippets/freehire-app.conf.bak.*   # release.sh never writes these; they are hand-made
cp /etc/nginx/snippets/freehire-app.conf.bak.<stamp> /etc/nginx/snippets/freehire-app.conf
nginx -t && systemctl reload nginx
```

## Checking for drift

`./deploy/check-drift.sh` diffs the host against this directory and prints what has moved
apart. It reads; it never writes to either side.
