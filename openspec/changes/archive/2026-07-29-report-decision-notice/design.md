## Context

`internal/report` owns the moderation queue: `File` stores a pending report, `ListPending`
serves the moderator queue, `Resolve` and `Dismiss` mark the decision, and `Resolve` may
soft-close the reported job through the narrow `JobCloser` seam. Nothing in that path
reaches the person who filed the report.

Two constraints shape the design:

- **The domain must not import AWS.** `internal/referral` set the precedent: it declares a
  one-method `EmailSender` locally and the concrete `*emailnotify.Client` is injected at the
  wiring site in `internal/handler/handler.go`, where SES is already constructed for
  referral pings and account mail.
- **An unconfigured channel is a soft skip, not a failure** (`docs/agents/notifications.md`).
  Every dev machine runs without SES; a decision must not fail there.

The full brainstormed design lives at
`docs/superpowers/specs/2026-07-29-report-decision-notice-design.md`.

## Goals / Non-Goals

**Goals:**

- A reporter learns what a moderator decided, in the moderator's own words when written.
- The moderator can tell whether the notice actually went out.
- No migration, no new external dependency, no new worker.

**Non-Goals:**

- No in-app notification centre or report-history page for the reporter. Email only.
- No retry queue — the notifications stack has none, and one failed notice does not justify
  inventing an outbox for this path.
- No Telegram channel, even though a report may carry `contact_telegram`. That field stays
  what it is: a way for a moderator to reach the reporter by hand.

## Decisions

### The note reuses `job_reports.review_reason`

The column exists (`text NOT NULL DEFAULT ''`) and is written only by dismiss today. Resolve
starts writing the moderator's note into it. No migration, and the audit trail for both
decisions stays in one place.

*Alternative considered:* a separate `moderator_note` column, keeping `review_reason`
internal. Rejected — two columns holding "why the moderator decided this" would drift, and
nothing reads `review_reason` as internal today (the reporter has no endpoint for it).

**The consequence must be handled, not just noted:** the column changes from an internal
annotation to user-facing text. The moderator UI labels the field as mailed to the reporter
at the point of entry. Rows written before this change are never mailed — the notice is sent
only at decision time.

### A `ReporterNotifier` seam beside `JobCloser`

```go
type ReporterNotifier interface {
    NotifyDecision(ctx context.Context, d Decision) error
}
```

`Decision` carries the recipient, the reported job, the original report, the moderator's
note, the outcome, and whether the job was closed. The service depends on the interface; the
`MailNotifier` in the same package implements it over a locally declared `EmailSender`.

*Alternative considered:* putting the mailer in `internal/emailnotify` as a sibling of
`AuthMailer`. Rejected — the referral precedent keeps outcome-specific copy next to the
domain that owns the outcome, and `emailnotify` stays the transport plus the two generic
mail shapes.

The notifier is attached with `WithNotifier` rather than a fourth `New` parameter, so
existing construction sites and tests are untouched and a nil notifier stays representable.

### Input structs for the two decisions

`Resolve(ctx, reviewerID, id, ResolveInput{CloseJob, NotifyReporter, Note})` and
`Dismiss(ctx, reviewerID, id, DismissInput{NotifyReporter, Reason})`, both returning
`Review{Report, Notified}`. Three positional booleans in a row is where call sites start
passing them in the wrong order.

### Order of operations, and what a failure means

1. Load the report with reporter and job context; reject anything not `pending`.
2. Resolve only: soft-close the job when asked. A close failure aborts **before** the mark —
   unchanged from today, so the report stays pending and the action is safe to retry.
3. Mark the decision, storing the note.
4. Notify, if asked and if a notifier is configured. A send failure is logged and surfaces as
   `Notified: false`; it never unwinds the decision.

Notifying after the mark is what keeps the message honest: a notice can never claim an
outcome the database does not hold. The reverse order would mail a decision that a failed
`UPDATE` then discarded.

### `GetReport` grows a join

The decision path needs the reporter's email and the job's slug and title. `GetReport` joins
`users` and `jobs` rather than issuing a second query, and `report.PendingReport` is renamed
`ReportDetail` — with `Get` returning it, the type is no longer specific to the pending
queue.

### `notify_reporter` on the wire

Absent means false, because Go zero-values the field and a `*bool` for one flag is
ceremony. The SPA always sends it explicitly with its checkbox on by default; an API-key
caller that wants a notice says so.

## Risks / Trade-offs

- **A moderator writes an internal aside into a field that now gets mailed.** → The UI labels
  the textarea as mailed to the reporter, and the notice is opt-out per decision.
- **A notice is lost with no retry.** → The response carries `notified`, and the queue warns
  when it is false, so the moderator can follow up by hand. An outbox for one message per
  moderation action is not worth its failure modes.
- **Mail volume follows moderation volume.** → Bounded by the review queue, which is capped
  at 500 pending rows and worked by hand.
- **Notes and reported details are attacker-influenced text rendered into HTML.** → Both go
  through `html/template` escaping; nothing is interpolated raw. The quoted report is cut on
  a rune boundary, so a Cyrillic or CJK report cannot end in half a character.
- **A decision can mail an unverified address.** A password-registered account can file a
  report before confirming its address, and the queue defaults to notifying — so a typo'd
  address hard-bounces. → Watch-item, not a gate: the same address already receives the
  verification code, so this adds no exposure SES does not already have. If bounce rates
  move, gate the notice on `users.email_verified`.

## Migration Plan

None. No schema change, no backfill, no new environment variable — the SES client and
`NOTIFY_EMAIL_FROM` are already wired for referrals and account mail. Deploying the code is
the whole rollout; rolling back stops the notices and leaves the stored notes harmless.

## Open Questions

None outstanding — content, triggers, failure behaviour, opt-out, and the moderator UI were
settled during brainstorming.
