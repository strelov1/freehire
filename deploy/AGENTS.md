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
