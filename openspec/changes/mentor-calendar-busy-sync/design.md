## Context

`mentor_busy_intervals` (migration 0145) and `Repository.ListBusy` already exist and are
already read by the slot engine — see proposal.md for why the table has stayed empty.
This design covers only how the sync itself is built. `internal/application/calsync`
already syncs a *candidate's* calendar into interview rows and is the closest existing
analog: same OAuth connector, same best-effort-per-user worker shape, same revocation
handling. It is not reused directly because it reads full events and matches them against
applications — a different job with a different privacy shape (see Decisions).

`gmail_connections` holds ONE Google grant row per user, with a `scopes text[]` column
recording what Google actually granted, checked by every existing incremental-consent
feature to decide what it may call. `CalendarScope` (`calendar.readonly`) already exists
in `gmailsync/connector.go` and is already requested by an unrelated candidate-side
feature (the interview calendar). This matters for the first decision below.

## Goals / Non-Goals

**Goals:**
- Keep `mentor_busy_intervals` current for every mentor who opts in, on a schedule.
- Never store more than an interval's bounds.
- Make one mentor's broken grant invisible to every other mentor's sync.

**Non-Goals:**
- Any UI to manage or disconnect the grant beyond the connect card (same non-goal
  `mentor-google-meet-link` already carries for its own grant).
- Syncing anything other than the primary calendar.
- Changing how the slot engine consumes the busy set — it already unions
  `mentor_busy_intervals` with confirmed bookings and does not need to know where either
  side came from.

## Decisions

### Free/busy, not events

`calsync` reads `events.list` and discards title/attendee/description in application
code because it needs the event's own identifier (`iCalUID`) to match it against an
application later. This feature has no such need — a busy interval either blocks a slot
or it doesn't, and the slot engine's own spec already says the response carries "no
title, attendee, or description" for it. Google's `freeBusy` endpoint returns exactly
that: a list of `{start, end}` periods, already merged across everything on the
calendar, with no way to identify or request any additional field. Asking Google for
strictly less data is a stronger guarantee than reading more and discarding it, so this
worker calls `freeBusy` and never `events.list`.

**Consequence for the storage key**: `mentor_busy_intervals`'s unique constraint is
`(mentor_id, source, external_id)`, written with an events-based sync in mind ("one row
per calendar event per mentor"). A free/busy period carries no identifier. This design
derives `external_id` deterministically from the interval's own bounds (a hash of
`starts_at|ends_at`), so the constraint's actual job — a re-sync updates rather than
duplicates — holds for intervals exactly as it would for events. Two independent busy
blocks that happen to share identical bounds collapsing into one stored row is not a
bug: they are indistinguishable busy time either way.

### A dedicated opt-in flag, because the scope is shared

Every existing incremental-consent feature in this codebase (Gmail, calendar-read,
mentor-calendar-write) can tell "did the user consent to THIS purpose" by checking
whether `scopes` contains a scope unique to that purpose — `CalendarEventsScope` exists
for no reason other than to make that true for the write grant. That trick is
unavailable here: Google has exactly one read scope for the whole calendar
(`calendar.readonly`), and this feature deliberately reuses it rather than asking for
anything narrower. A mentor who already granted `calendar.readonly` for the unrelated
candidate-side interview calendar would therefore already show that scope in `scopes`,
and gating on scope presence alone would enroll them in busy-sync without their consent
— exactly what the spec's "unrelated Google connection does not imply this grant"
requirement forbids.

The fix is a new boolean column, `gmail_connections.mentor_busy_sync_opted_in`, set only
by this flow's own callback on success. The sync worker's eligibility query requires
both `mentor_busy_sync_opted_in` and `calendar.readonly ∈ scopes` — the flag records
*consent for this purpose*, the scope check confirms the grant can still actually be
used, and the two cannot drift into "consented but scope check fails" without it being a
real revocation the ordinary `needs_reconsent` handling already catches.

### Reconcile by replacing the synced window, not by diffing rows

Each run deletes every stored interval for a mentor whose `starts_at` falls inside the
run's sync window and re-inserts what `freeBusy` currently reports for that same window,
in one transaction. This is simpler than computing an add/update/remove diff and equally
correct: the worker owns every row with `source = 'google_calendar'` unconditionally (no
other writer ever touches that source), so replacing the window is not lossy, and the
per-mentor transaction means a mentor's busy set is never read by a concurrent booking
attempt in a partially-reconciled state — `ListBusy` sees either the run's whole
previous answer or its whole new one, never a mix.

### A fixed forward window, not each mentor's own horizon

A mentor's booking horizon is a per-mentor, per-session-type `SessionParams` value, not
a constant — joining it in would make the sync window itself part of the mentor's live
configuration. Reading a fixed, generous window (60 days forward, no backward window —
past busy time cannot block a future booking) costs one extra, unremarkable API call's
worth of data for a mentor whose horizon is shorter, and needs no join or per-mentor
branching. `ListBusy` already narrows to whatever window the slot engine actually asks
about, so over-fetching here is inert.

## Risks / Trade-offs

- **[Risk]** A mentor with a very short horizon has this worker read further ahead than
  it needs to, for a small, constant cost. → Not mitigated; the constant window's
  simplicity is worth more than the saved API cost (see previous decision).
- **[Risk]** The transactional replace-the-window reconcile means a mentor with many
  scattered busy periods still costs one delete-and-reinsert per run, not an
  incremental patch. → Acceptable: the run is bounded by the number of *connected*
  mentors, expected to be small for the foreseeable future, unlike `cmd/ingest`'s
  million-row scale.
- **[Risk]** `freeBusy` reports a single calendar's busy time as Google understands it,
  including declined or tentative events depending on the calendar's own settings —
  this worker cannot distinguish "tentative" from "busy" because free/busy does not
  expose it. → Accepted: the same imprecision Google's own free/busy UI carries, and
  finer-grained control would require reading events after all, which reopens the
  privacy trade-off this design avoids.

## Migration Plan

Additive only: a new `gmail_connections.mentor_busy_sync_opted_in boolean NOT NULL
DEFAULT false` column, and no change to `mentor_busy_intervals`'s existing schema (this
sync writes to columns that already exist). The new `cmd/mentor-busy-sync` worker and
its timer are provisioned on the host the same way `cmd/mentorship-remind` was —
`deploy/systemd/freehire-mentorship-remind.*`'s own AGENTS.md note that these are not
installed by `release.sh` applies here too. Rollback is deleting the timer and column;
no data migrated in requires migrating back out.
