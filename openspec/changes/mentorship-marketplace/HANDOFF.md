# Handoff: mentorship marketplace

Written 2026-09-07, at the point where the backend is complete and the frontend is not
started. Read `proposal.md` for why, `design.md` for the decisions, `tasks.md` for the
checklist. This file is the short version plus the things a new session would otherwise
have to rediscover.

## Where the work is

- **Branch**: `worktree-mentorship-marketplace`
- **PR**: https://github.com/strelov1/freehire/pull/2626 — 16 commits, 65 files, all 18 CI
  checks green, `mergeStateStatus: CLEAN`. Not merged; merging is the user's call.
- **Worktree**: `.claude/worktrees/mentorship-marketplace` (created with EnterWorktree)
- **Reference implementation read while designing**: `/Users/i_strelov/Projects/cal.com`,
  a shallow read-only clone. Never build or run it. Useful files:
  `packages/features/schedules/lib/date-ranges.ts` and `.../slots.ts`.

39 of 53 tasks are `[x]`. What remains is listed at the bottom.

## What exists

**Backend, complete and tested.** New package `internal/engage/mentorship` (slot engine,
profile, booking, reminders, notifications, iCalendar generation), migration
`0145_mentorship.sql` (six tables), 29 sqlc queries, 20 HTTP routes, cron worker
`cmd/mentorship-remind` with its systemd unit and timer.

Read `internal/engage/mentorship/AGENTS.md` before changing anything in that package. It
records what is always true there, and most of it was learned by getting it wrong first.

**Nothing in `web/`, `design-system/` or `extension/`.** Zero frontend changes; the split
was deliberate.

## The API the screens will use

Public (no auth, rate-limited on their own budget):

```
GET  /api/v1/mentors                    ?company= &topic= &language= &limit=
GET  /api/v1/mentors/:slug
GET  /api/v1/mentors/:slug/slots        ?from= &to= &timezone=
```

Seeker (cookie or API key):

```
POST /api/v1/mentors/:slug/bookings     {starts_at, timezone, note, job_id}
GET  /api/v1/me/mentorship/sessions
GET  /api/v1/me/mentorship/sessions/:id
POST /api/v1/me/mentorship/sessions/:id/cancel   {reason}
PUT  /api/v1/me/mentorship/sessions/:id/review   {rating, comment}
```

Mentor cabinet:

```
GET    /api/v1/me/mentorship/profile
POST   /api/v1/me/mentorship/profile             {company_slug, slug, name, headline, bio,
                                                  topics[], languages[], timezone,
                                                  session_minutes, buffer_before_minutes,
                                                  buffer_after_minutes, notice_minutes,
                                                  horizon_days, meeting_url}
PUT    /api/v1/me/mentorship/profile
DELETE /api/v1/me/mentorship/profile             (withdraw)
POST   /api/v1/me/mentorship/profile/pause       {paused}
GET    /api/v1/me/mentorship/availability
PUT    /api/v1/me/mentorship/availability/weekly {rules: [{weekday, start, end}]}
POST   /api/v1/me/mentorship/availability/overrides {date, start, end}
DELETE /api/v1/me/mentorship/availability/:id
GET    /api/v1/me/mentorship/bookings
```

Moderation (moderator gate):

```
GET  /api/v1/mentorship/profiles
POST /api/v1/mentorship/profiles/:id/decide      {status: approved|rejected}
```

### Wire shapes the frontend must not get wrong

- **A slot carries three things**: `starts_at`/`ends_at` (absolute UTC), `local_start`/
  `local_end` (RFC 3339 in the viewer's zone), and `utc_offset`. All three are needed —
  the instant is what gets booked, the wall clock is what a person reads, and **the offset
  is the only thing distinguishing the two slots that share a label on the autumn
  daylight-saving transition**. Render the offset, or one October evening shows "02:00"
  twice with no way to tell them apart.
- **`meta.timezone` says which zone was actually used.** An unrecognised one falls back to
  UTC. Do not assume the response is in the zone you asked for.
- **Send the browser's zone**: `Intl.DateTimeFormat().resolvedOptions().timeZone`.
- **Sessions come back split**: `{"data": {"upcoming": [...], "past": [...]}}`.
- **The public profile has no `meeting_url`.** It is a live room; only a booked party gets
  it, on their booking.
- **`meta.ignored_params`** appears on the directory when a query parameter was not read.
- **An availability rule carries an `id`** — the delete route needs it. A weekly rule has
  `weekday`, a dated one has `date`; never both. `closure: true` marks a dated rule with an
  equal start and end, which CLOSES that day — render it as "away", not as "10:00–10:00".
- **Times are `HH:MM`, and `24:00` is valid** — it means "until midnight" without spilling
  onto the next date.

## What is left

**Frontend — 7 screens, group 8 in tasks.md.** Directory with filters; public profile with
the booking calendar; booking confirm/cancel and the seeker's session list; mentor cabinet
under `/my/` (profile, weekly schedule, date overrides, own bookings); moderation queue
beside the referral one; review submission; and the entry point from a vacancy and company
page — coordinate its placement with the open `job-page-cta-hierarchy` change rather than
adding a fourth button independently.

Grep `web/` for an existing implementation before designing any of these.

**Deploy — two manual steps.** `release.sh` builds the API, not every command in `cmd/`, so
`mentorship-remind` needs building on the host; and it never touches a unit, so
`deploy/systemd/freehire-mentorship-remind.{service,timer}` are copied across by hand.
Until both are done the feature works and reminders simply never fire.

**The backend is finished.** The two items this file used to list as outstanding — the
`_ "time/tzdata"` import in `cmd/server` (task 1.10) and the test that a silenced account
still hears about its own session (task 5.3) — are done. Everything left below is the
frontend and the two manual deploy steps.

**Not in scope, seams named and empty**: payments, and the Google Calendar free/busy sync
(`mentor_busy_intervals` exists and stays empty until that change).

## What was decided, and would be wrong to undo

- **The mentor is NAMED.** `mentors.display_name` is required. This is the whole thing that
  makes it not `internal/engage/referral`, which keeps its insider anonymous on purpose.
- **No avatar yet**, and `specs/mentor-profile/spec.md` says why rather than leaving the
  requirement half-met: a public image endpoint has its own caching, sizing and abuse
  surface, and the account's stored headshot is a CV photo a mentor may not want here. It
  belongs with the screens that show it — so it is the frontend PR's decision to make.
- **Withdrawal MARKS the profile**, it does not delete it. Bookings and reviews cascade off
  that row.
- **Slots are never stored**, and the grid is anchored to the mentor's schedule, never to
  the current instant.
- **The slot cache is not explicitly invalidated** — a one-minute expiry bounds it, and the
  booking path does not read it.

## Traps this work walked into, recorded so the next session does not

1. **A fix verified against the caller you wrote the test for is not verified.** The slot
   grid bug was found, fixed, mutation-tested and still shipped broken: every test supplied
   a window starting at midnight, and the real HTTP handler supplies `now`. Call the code
   the way production calls it.
2. **A fake repository describes the contract; it cannot prove it.** Three defects hid
   behind ours — a confirmation that reached nobody (the fake filled fields the SQL does
   not return), a withdrawal that erased history (a fake has no cascades), and a paused
   mentor served from cache. Anything that is a property of the schema belongs in
   `internal/platform/db/mentorship_integration_test.go`.
3. **A test can hold a bug in place as firmly as a feature.** The reminder test booked a
   session three hours out and *required* the "in 24 hours" reminder to fire.
4. **Run whole-module guards whole.** `deadcode` was run locally and its output grepped for
   "mentorship"; the breakage was in `pgerr`, and CI found it.
5. **`go test` caches by input hash, and a filesystem-scanning guard is not in that hash.**
   The layering test reported `ok (cached)` on a package it had never seen. Use `-count=1`.
6. **`deploy/AGENTS.md` is a record of things that have already gone wrong.** It said mail
   credentials live only in `.env.notify`, and this work walked into that anyway. Read it
   before writing a unit.
