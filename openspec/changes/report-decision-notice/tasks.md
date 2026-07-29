## 1. Data access

- [x] 1.1 Join reporter and job context into `GetReport` and store the note in
      `MarkReportResolved` (`internal/db/queries/job_reports.sql`), then `make sqlc`
- [x] 1.2 Rename `report.PendingReport` to `report.ReportDetail`, return it from
      `Repository.Get`, and map the new columns in `internal/report/repository.go`

## 2. Decision use cases

- [x] 2.1 Add the `ReporterNotifier` seam, the `Decision` shape, and `WithNotifier` to
      `internal/report`
- [x] 2.2 Move `Resolve` and `Dismiss` to `ResolveInput`/`DismissInput` and a `Review`
      return, storing the moderator's note
- [x] 2.3 Notify after the decision is marked, only when asked and when a notifier is
      configured; a send failure or a nil notifier yields `Notified: false` and leaves the
      decision standing

## 3. The notice itself

- [x] 3.1 Build `MailNotifier` over a local `EmailSender` with the three outcomes
      (resolved+closed, resolved+open, dismissed), escaping the note and the quoted details
- [x] 3.2 Degrade a blank note to a generic statement instead of an empty quotation

## 4. HTTP surface

- [x] 4.1 Accept `note`/`notify_reporter` on resolve and `notify_reporter` on dismiss, and
      return `notified` in `internal/handler/reports.go`
- [x] 4.2 Hand the existing SES client to the report service in
      `internal/handler/handler.go`, leaving the service functional without it

## 5. Moderator queue

- [x] 5.1 Carry `note`, `notify_reporter`, and `notified` through `web/src/lib/api.ts` and
      `web/src/lib/types.ts`
- [x] 5.2 Replace `window.prompt` in `ReportQueue.svelte` with an inline note field labelled
      as mailed to the reporter, plus a `Notify reporter` checkbox on by default
- [x] 5.3 Warn in the queue when a decision was recorded but the notice did not go out

## 6. Verification

- [x] 6.1 Extend `internal/handler/reports_integration_test.go`: `notified` in the decision
      responses, and the note stored as `review_reason` for both decisions
- [x] 6.2 Run `go build ./... && go vet ./... && go test ./...`, the integration tag suite for
      the touched package, and the web lint/build gates
