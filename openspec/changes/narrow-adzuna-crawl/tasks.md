## 1. Order the crawl by date and bound it by freshness

- [x] 1.1 Write the failing tests first in `internal/ingest/sources/adzuna_test.go`: the built
  page URL carries `sort_by=date`; it carries `max_days_old` with the configured window; the
  existing country/category/page/`results_per_page` assertions still hold. The URL builder
  (`adzunaPageURL`) is a pure function, so these are exact-value assertions, not a live call.
- [x] 1.2 Add `sort_by=date` and `max_days_old` to `adzunaPageURL` in
  `internal/ingest/sources/adzuna.go`, with named constants beside `adzunaMaxPages` and a
  comment recording WHY each earns its place: the ordering because the platform's default is
  relevance and stable between runs (the 2026-02-10-first-result measurement), the window
  because it is what makes the loop's existing empty-page exit reachable on a quiet board.
- [x] 1.3 Cut `adzunaMaxPages` from 40 to 15 and update its comment: the bound is now one
  leg of a request budget (boards × pages × runs/day), not a standalone "how deep is
  sensible" figure. State the arithmetic — 4 boards × 15 pages × 4 runs = 240/day against a
  250/day ceiling — so a later edit to any one of the three sees what it is spending.
- [x] 1.4 Test that a board whose window returns fewer postings than the page budget stops on
  the empty page rather than issuing the remaining requests. This is the behaviour
  `max_days_old` exists for, and it is the one that is invisible in a URL assertion.
- [x] 1.5 `gofmt -w` the touched files, then `go build ./...`, `go vet ./...`, `go test ./...`.

## 2. Close the ageing tail by age

- [x] 2.1 Add `"adzuna"` to `expireDespiteRegisteredPrefixes` in `cmd/liveness/main.go`, and
  extend that variable's comment: it currently explains the whatjobs case (a per-country CPC
  family whose URL is a billing landing page). Adzuna is the same classification reached by a
  different route — a tracking redirect behind bot protection — and the comment should carry
  the 403 measurement, since the next reader's obvious question is "why not just probe it?".
- [x] 2.2 Test in `cmd/liveness/main_test.go` that `matchingProviders` resolves `adzuna` from
  a registry containing it, and that the prefix does not accidentally match a different
  provider.

## 3. Stop a missing credential from silently changing the mechanism

- [x] 3.1 Write the failing test first: a registry with no `adzuna` entry, `adzuna` listed in
  `expireDespiteRegisteredPrefixes`, and the worker refusing to run.
- [x] 3.2 Add the guard in `run()` beside the existing `probeDespiteRegistered` drift guard: a
  configured prefix that matches no registered provider stops the run and names the prefix.
  Mirror the existing guard's log wording so the two read as one family.
- [x] 3.3 Correct the comment on `expireDespiteRegisteredPrefixes` that currently states "no
  drift guard needed, since membership follows atsProviders by construction". That reasoning
  covers a market being ADDED, which is the case it was written for; it does not cover a
  member being ABSENT, which is what a missing credential produces. Record the distinction
  rather than deleting the sentence — the construction argument is still why the list holds
  prefixes rather than names.
- [x] 3.4 Verify the premise on prod before relying on it: `/opt/freehire/.env` carries
  `ADZUNA_APP_ID` and `deploy/systemd/freehire-liveness.service` reads that file, so the
  guard passes today. Confirmed 2026-09-06; the task is to re-confirm at deploy time, since a
  guard that fires on a healthy host stops the whole liveness run, not just Adzuna's part.
- [x] 3.5 `go test ./cmd/liveness/`, then `go vet -tags=integration ./...`.

## 4. Slow the timer

- [x] 4.1 Change `deploy/systemd/freehire-ingest@adzuna.timer` from `OnCalendar=*:22:00`
  (hourly) to four runs a day, keeping the off-the-hour minute and the existing
  `RandomizedDelaySec` so it does not land with the rest of the fleet.
  - **Landed as `00/6:22:00`, and the two forms it is not are worth recording.** `*/6:22:00`
    — the shape the minute/second fields would suggest — does not parse at all, which is the
    harmless failure. `*:22:00/6` DOES parse, as every hour at :22 repeating on the *second*,
    so it would have shipped the original hourly cadence while reading in review like a
    change. Nothing in this repository's build looks at a `.timer`, so it would have surfaced
    only as an Adzuna request count that never fell. Verified against `systemd-analyze
    calendar --iterations=4` on the host, and `systemd-analyze verify` accepts the unit.
- [ ] 4.2 Copy the unit to the host and `systemctl daemon-reload` + restart the timer.
  `release.sh` flips the app and never touches a unit, so this step does not ship itself —
  the change is half-applied until it is done, and the half that is live (a 15-page adapter
  running hourly) is still ~4× the intended budget.
- [ ] 4.3 Confirm with `systemctl list-timers freehire-ingest@adzuna` that the next
  elapse matches the new cadence, and with `./deploy/check-drift.sh` that nothing else in
  `deploy/` has drifted from the host in the meantime.

## 5. Verify against the platform, not against the tests

- [ ] 5.1 After deploy, count a day's real requests rather than trusting the arithmetic: the
  ingest logs report pages per board per run. Confirm the daily total is under 250. The
  arithmetic assumes every board spends its full budget; `max_days_old` should make the quiet
  boards spend less, so the measured figure should come in BELOW 240, and a figure at or above
  it means the early exit is not firing.
- [ ] 5.2 Confirm intake moved in the intended direction: `jobs.created_at` for `source =
  'adzuna'` grouped by day, compared against the 7-day pre-change mean of ~6,300/day. Expect
  it to roughly double. A figure that does NOT move is the signal that `sort_by=date` is not
  reaching the API — check a logged request URL before looking anywhere else.
- [ ] 5.3 Watch the Adzuna slice shrink and record where it settles. Expect 301k to fall
  toward 150-180k over several weeks, faster at first (the 14-day sweep reaches postings
  before the 45-day age rule does). Note the actual figure in this file when it stabilises —
  the estimate is derived from an age histogram, not from having watched it happen.

## 6. Refresh the duplicate markers once

- [ ] 6.1 Run `REINDEX_DEDUP_ONLY=1` on prod so aggregator suppression re-marks the 142,229
  open Adzuna postings whose company is already held first-party. No need to pause
  `search-drain` for it — the marker-only pass builds no Meilisearch client and runs no
  rebuild. It is an operational step, not a code dependency: nothing in sections 1-4 waits on
  it, and skipping it only leaves search showing duplicates it already shows today.
- [ ] 6.2 Record how many markers it actually set. Suppression needs a TITLE twin, not merely
  a covered company, so the reachable subset is smaller than 142,229 — 35,268 Adzuna postings
  carry an aggregator marker today, and the gap between those figures is what this run moves.

## 7. Close out the issue

- [ ] 7.1 Post the measurement and the verdict to issue #1759 — step 4 asked whether Adzuna's
  remainder is exclusive, and it is: 20,350 companies and 81,730 open postings exist in the
  catalogue through Adzuna and nowhere else. Adzuna is kept and narrowed, not deprecated.
- [ ] 7.2 Note in #1759 that its step 2 instructions are stale: `sources/` was retired in
  #2406 and `cmd/harvest-boards` now writes to the `boards` table. A reader following the
  issue's text lands on a directory that no longer exists.
- [ ] 7.3 Note that batch 2/2 (`breezy`, `freshteam`, `personio`, `trakstar`, `workable`) was
  promised in a comment on 2026-08-12 and never landed — no PR references it. Either finish it
  or say plainly that it was dropped; a half-finished batch recorded as "to follow" reads as
  in-progress forever.

## 8. Out of scope, recorded so it is not lost

- [ ] 8.1 `cmd/hydrate-adzuna-description` fetches the same `…/land/ad/<id>` URLs every 30
  minutes and meets the same 403 this change measured. Its own source comment records "no
  JobPosting block" as the majority failure mode of its first production runs, which is what
  an Access Denied page looks like to it. Measure its success rate; if it is near zero, it is
  spending a request every 3.6 seconds for nothing. Separate change.
