# deploy

The production host's systemd units and operator scripts, as they run on host-2. Not Go,
not built, not imported by anything — this directory is a **record**, and the only reason
it exists is that the machine was the sole copy.

Snapshot taken 2026-09-01 from `/etc/systemd/system/freehire-*` and `/opt/freehire/bin/*.sh`.

## What is here

```
systemd/   337 files — 46 .service, 286 .timer, 5 drop-in directories
bin/        13 operator scripts (release, backups, alerting, ingest slotting)
```

`systemd/freehire-ingest@.service` is one template; the 255 `freehire-ingest@<board>.timer`
files beside it are per-source schedules, which is why the timer count dwarfs everything
else. Nothing generates them — a new board means a new timer file, by hand.

## Always true

- **Nothing here deploys itself.** `release.sh` builds and flips the app; it does not touch
  units or the scripts in this directory. Changing a file here is half the job — the other
  half is copying it to the host and running `systemctl daemon-reload`. Treat git as the
  truth and the host as the copy, not the other way round, or this snapshot rots into
  fiction within a month.
- **Billing has three manual dashboard steps, and the units are useless without them.**
  `freehire-billing-sync` and the webhook route both read `REVENUECAT_API_KEY`,
  `REVENUECAT_WEBHOOK_SECRET`, `REVENUECAT_ENTITLEMENT` and `BILLING_CHECKOUT_URL` from
  `/opt/freehire/.env`. Getting them means, in the provider's dashboard: registering the
  webhook at `https://freehire.me/api/v1/billing/revenuecat/webhook` and **enabling HMAC
  signing** on it (the handler refuses an unsigned delivery — there is no fallback);
  minting a **secret** `sk_` key, since a public one is refused on the subscriber
  endpoint; and creating the Web Billing paywall, whose token is what
  `BILLING_CHECKOUT_URL` holds.

  **`REVENUECAT_PROJECT_ID` is required** (`proja56a40fc`): the code speaks API v2 and
  every v2 call is scoped to a project. Its absence leaves billing disabled, which is the
  safe direction but looks exactly like "not deployed yet".

  **`REVENUECAT_ENTITLEMENT` holds BOTH names of the entitlement**, comma-separated:
  `freehire Pro,entl58d5471b41`. The provider has a human lookup key and an internal id for
  one entitlement and names it with one of them in the customer payload; configuring both
  is what stops that being discovered from an incident. And note it is **not** `pro` — That is the lookup key the
  project actually carries, and the package's default would match no entitlement at all —
  which does not fail, it resolves every paying subscriber to the free plan. The value has
  a space in it; systemd's `EnvironmentFile` strips the surrounding quotes and delivers it
  whole, verified on the host rather than assumed. With none of them set every billing route is simply
  not mounted and the timer is a no-op that never opens the pool, so the units are safe to
  install before the dashboard is ready — they are just inert.
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
  shellcheck over every tracked `*.sh`, so these 13 are covered the moment they are
  committed. The findings that came with them were all style-level and are suppressed
  inline with the reason beside them — none was a defect.

## Checking for drift

`./deploy/check-drift.sh` diffs the host against this directory and prints what has moved
apart. It reads; it never writes to either side.
