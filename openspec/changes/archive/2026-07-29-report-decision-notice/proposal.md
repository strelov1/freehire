## Why

A user who reports a problem with a vacancy never learns what came of it: the moderation
queue lets a moderator resolve or dismiss the report, and the decision reaches no one. The
case that prompted this — a reporter flagged a job listed as Remote that its source posts as
Hybrid, the classification was fixed, and the reporter has no way to know. Reporting is the
cheapest data-quality signal the product has, and silence trains people to stop sending it.

## What Changes

- A moderator decision (resolve or dismiss) emails the reporter what happened.
- Both decisions accept a moderator's free-text note, which is quoted in that email. Resolve
  gains the note; dismiss already carries a reason and now sends it. The note is stored in
  the existing `job_reports.review_reason` column — no migration.
- **BREAKING (internal semantics):** `review_reason` stops being an internal annotation and
  becomes user-facing text. The moderator UI must label it as mailed to the reporter.
- Both decisions accept `notify_reporter`, so spam and duplicate reports close quietly.
- The decision response carries `notified`, telling the moderator whether the email actually
  went out. A send failure never unwinds the decision.
- The moderator queue replaces `window.prompt` with an inline note field and checkbox.

## Capabilities

### New Capabilities
<!-- None. The notice is part of the existing report review flow, not a separate capability. -->

### Modified Capabilities
- `job-report`: resolve and dismiss gain a moderator note, an opt-out, and a mailed notice to
  the reporter; the decision response reports whether that notice was delivered.

## Impact

- `internal/report` — a `ReporterNotifier` seam beside the existing `JobCloser`, input
  structs for the two decisions, and the `MailNotifier` that renders the three outcomes.
- `internal/db/queries/job_reports.sql` — `GetReport` joins reporter and job context;
  `MarkReportResolved` stores the note. Regenerated with `make sqlc`. No migration.
- `internal/handler/reports.go`, `internal/handler/handler.go` — request/response fields and
  the SES wiring already built for referrals and account mail.
- `web/src/lib/components/ReportQueue.svelte`, `web/src/lib/api.ts`, `web/src/lib/types.ts` —
  the inline decision form and the new fields.
- Outbound mail volume: one message per moderator decision, bounded by the review queue.
