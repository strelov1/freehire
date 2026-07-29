# Report decision notice — design

**Date:** 2026-07-29
**Status:** approved, ready for planning

## Problem

A user files a report on a job and never hears back. The moderation queue at
`/moderation?tab=reports` lets a moderator resolve (optionally soft-closing the vacancy) or
dismiss a report, but the decision is invisible to the person who raised it. The concrete
case that prompted this: a reporter flagged a job listed as Remote that the source posts as
Hybrid, the classification was fixed, and the reporter has no way to learn that.

Reporting is the cheapest data-quality signal the product has. Silence trains people to stop
sending it.

## Goal

When a moderator decides a report, email the reporter what happened — in the moderator's own
words when they choose to write them.

## Non-goals

- No in-app notification centre or report history page for the reporter. Email only.
- No retry queue. A failed send is surfaced, not re-attempted (the notifications stack has no
  retry either).
- No Telegram channel for this notice, even though a report may carry `contact_telegram`.
  That field stays what it is: a way for a moderator to reach the reporter manually.

## Decisions

| Question | Decision |
|---|---|
| Content | Moderator's free-text note, quoted in the mail. Blank note falls back to a generic sentence. |
| Triggers | Both resolve and dismiss. A dismissal without a word reads as being ignored. |
| Send failure | The decision stands; the response carries `notified: false` and the queue warns. |
| Opt-out | A `Notify reporter` checkbox, on by default, so spam and duplicate reports close quietly. |
| Moderator UI | Inline block inside the queue row, not a modal. |

## Data

No migration. `job_reports.review_reason` already exists (`text NOT NULL DEFAULT ''`) and is
written only by dismiss today; resolve starts writing the moderator's note into the same
column.

**The column changes meaning.** Until now `review_reason` was an internal annotation the
reporter had no endpoint to read. Once it is quoted in the mail it is user-facing text, and
the UI must say so at the point of entry — otherwise a moderator will one day write an
internal aside into a field that gets mailed to a stranger.

Two query changes in `internal/db/queries/job_reports.sql` (then `make sqlc`):

- `GetReport` joins `users` and `jobs` so the decision path has the reporter's email and the
  job's slug and title without a second round trip.
- `MarkReportResolved` sets `review_reason` alongside the status, mirroring
  `MarkReportDismissed`.

`report.PendingReport` is renamed `report.ReportDetail` — with `Get` returning it, the type
is no longer specific to the pending queue. `ListPending` returns the same type.

## Domain (`internal/report`)

A second narrow seam beside `JobCloser`, following the `referral.ChannelPinger` precedent:

```go
// ReporterNotifier tells the person who filed a report what a moderator decided.
type ReporterNotifier interface {
    NotifyDecision(ctx context.Context, d Decision) error
}

// Decision is everything the notice needs: who to reach, what was reported, what happened.
type Decision struct {
    Email     string
    JobTitle  string
    JobSlug   string
    Reason    string // the report's controlled-vocabulary reason
    Details   string // the reporter's own words, quoted back
    Note      string // the moderator's note; may be empty
    Outcome   string // "resolved" | "dismissed"
    JobClosed bool
}
```

The use cases take input structs rather than growing positional parameters:

```go
type ResolveInput struct {
    CloseJob       bool
    NotifyReporter bool
    Note           string
}

type DismissInput struct {
    NotifyReporter bool
    Reason         string
}

// Review is the outcome of a moderator decision: the updated report and whether the
// reporter was actually reached.
type Review struct {
    Report   Report
    Notified bool
}
```

Ordering and failure handling, identical for both paths:

1. Load the report (with reporter and job context); reject anything not `pending`.
2. Resolve only: soft-close the job when asked. A close failure aborts before the mark, as
   today — the report stays pending and the action is safe to retry.
3. Mark the decision, storing the note in `review_reason`.
4. Notify, if asked and if a notifier is configured. **A send failure never unwinds the
   decision**: it is logged and reported as `Notified: false`.

A nil notifier (SES unconfigured — every dev machine) is a soft skip, not an error. This
mirrors `notify.Router`'s `ErrChannelNotConfigured` handling: an unconfigured channel must
never fail the operation it decorates.

`Service` gains the notifier through a `WithNotifier` option rather than a fourth `New`
parameter, so tests and the no-SES wiring construct the service unchanged.

## Mail (`internal/report/notifier.go`)

`MailNotifier` implements `ReporterNotifier` over a locally declared one-method
`EmailSender`, exactly as `referral` does, so the domain package never imports the AWS
dependency graph. `*emailnotify.Client` satisfies it at the wiring site.

Three outcomes over one template pair (HTML + text):

| Outcome | Subject | Body lead |
|---|---|---|
| resolved, job closed | `We removed the job you reported` | the vacancy is off the listings |
| resolved, job open | `We looked into your report on "<title>"` | `What we did:` + note |
| dismissed | `Your report on "<title>" — no change` | `We left it as is:` + reason |

Every mail quotes the reporter's original details (truncated to a display cap), links to the
job page under `FrontendOrigin`, and closes with the site footer used by the other
transactional mails. An empty note degrades to a generic sentence rather than an empty
quote block.

The note and the quoted details are moderator- and user-authored text rendered into HTML —
both go through `html/template` escaping. No note is ever interpolated raw.

## API

`reportResponse` gains `"notified": bool`.

- `POST /reports/:id/resolve` — body `{close_job, note, notify_reporter}`
- `POST /reports/:id/dismiss` — body `{reason, notify_reporter}`

`notify_reporter` defaults to false on absence because Go zero-values it; the SPA always
sends the field explicitly, and an API-key caller opting in must say so. Both routes stay
moderator-gated.

## SPA (`web/src/lib/components/ReportQueue.svelte`)

`window.prompt` is replaced. Each action button expands an inline block in the row:

- a textarea for the note, labelled *This note is emailed to the reporter*
- a `Notify reporter` checkbox, checked by default
- confirm and cancel buttons, with only one row expanded at a time

On success the row is dropped from the queue as it is today. When the response carries
`notified: false` while the moderator asked to notify, the view shows a warning line:
the decision was recorded but the email did not go out.

`api.resolveReport` / `api.dismissReport` and the `Report` type in `web/src/lib/types.ts`
pick up the new fields.

## Wiring (`internal/handler/handler.go`)

The SES client is already built in the referral block and shared with `AuthMailer`. The same
client is handed to the report service there. Without SES the service is constructed exactly
as now and every decision reports `notified: false`.

## Testing

Service, with a fake notifier and fake repo (no database):

- the notice is sent after the decision is marked, never before
- a notifier failure leaves the report decided and returns `Notified: false`
- `NotifyReporter: false` never calls the notifier
- a nil notifier does not panic and returns `Notified: false`
- resolve carries `JobClosed` and the note into the `Decision`; dismiss carries the reason
- the existing guarantees still hold: a close failure aborts before the mark, a non-pending
  report is `ErrAlreadyDecided`

Mail rendering, with a fake sender: the three outcomes produce their subjects and bodies, an
empty note degrades to the generic sentence, and HTML in a note or in the reporter's details
is escaped.

Handler integration (`internal/handler/reports_integration_test.go`): the decision responses
carry `notified`, and the note lands in `review_reason` for both resolve and dismiss.

## Announcement

A user-facing change, so a `/blog` changelog entry is offered once it ships
(`web/src/posts/`), per the repository convention.
