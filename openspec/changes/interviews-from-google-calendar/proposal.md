## Why

The tracking calendar can now say what happened to an application and when. It cannot say
the one date a candidate actually needs to keep: when the interview is. Nothing in the
system holds it. `mailclassify` has `interview_invitation`, but that signal means an
invitation *arrived* — dated by `emails.received_at`, not by the meeting. The rehearsal
preset reads the invitation's body for context and never extracts a time.

The date does exist, twice over: in the `text/calendar` part the ATS attaches to its
invitation, and in the candidate's own calendar once they accept. This change reads the
second and uses the first to link it.

## What Changes

- **A second Google consent**, for `calendar.readonly`, offered on its own. Connecting
  Gmail must not silently ask for a calendar; `include_granted_scopes` already makes the
  returned refresh token cover both grants.
- **`cmd/cal-sync`** — a run-once cron worker mirroring `cmd/gmail-sync`: per user,
  best-effort, marking `needs_reconsent` on a token failure and continuing.
- **Only matched meetings are stored.** The sync reads a ±90-day window in memory and
  persists a row only for an event it can attach to an application. A personal event is
  never written anywhere — the privacy boundary is structural, not a promise.
- **The deterministic tier is the `iCalUID`.** The ATS invitation carries a
  `text/calendar` part whose `UID` is the meeting's own identity, and that email is
  already linked to an application. Capturing the UID at ingest closes the chain
  exactly: mail → application, mail → meeting, therefore meeting → application.
  A company name in the title or an organiser's domain produces a *suggestion* the
  candidate confirms, never a link — the asymmetry `mailmatch` already holds.
- **The meeting lives in its own table, not in the ledger.** A scheduled meeting is
  mutable — it gets moved and cancelled — and it points forward, while
  `application_events.occurred_at` means "when this happened" and every day calculation
  reads it. The ledger instead records `interview_scheduled` at the moment the
  scheduling was observed, which is a fact about the past.
- **The calendar gains a second layer**: what happened, and what is arranged. A
  scheduled meeting is drawn as arranged rather than as observed.

## Capabilities

### New Capabilities
- `interview-schedule`: capturing an interview's time and attaching it to an
  application — the calendar grant, the sync window, what may and may not be stored,
  the deterministic UID tier and the suggestion tier below it, and what a reschedule or
  a cancellation does.

### Modified Capabilities
- `tracking-calendar-view`: the calendar shows scheduled meetings alongside recorded
  events, drawn so the two cannot be mistaken for each other. Additive — no existing
  requirement changes.

## Impact

- **New**: `internal/calsync` (connector scope, reader, worker), `internal/calmatch`
  (the tiers), `cmd/cal-sync`, migration `0070`.
- **Modified**: `internal/gmailsync/connector.go` (a second consent URL and scope set),
  `internal/mailingest` + `internal/gmailsync` (capture the `text/calendar` part's UID),
  `internal/appevent` (the `interview_scheduled` kind), the tracking calendar view.
- **Schema**: `emails.ical_uid`, `gmail_connections.scopes`, and a new
  `application_interviews` table. Migration `0070` — `0069` is the last applied on
  production, verified against its ledger.
- **Blocked on Google, and honestly so**: the OAuth app is unverified and holds a
  restricted scope, so the grant reaches test users only until verification. The
  connect surface must say that rather than failing opaquely.
