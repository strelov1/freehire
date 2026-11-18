## Why

An employer on freehire can publish a vacancy that takes applications here
(`add-in-app-apply`), but each application only arrives as an email. Nothing on freehire shows
it to them again, nothing records what they decided, and the candidate learns nothing unless
the employer writes back by hand. That is a job board that forwards mail, not a place to hire
from: the employer keeps the real process in their inbox or a spreadsheet, and the candidate
waits in silence — the experience freehire exists to end.

The candidate is already inside the same product, tracking the application. So the employer's
decision can reach them without anyone writing a letter, which no external ATS can offer.

## What Changes

- **An applicant inbox** in the employer workspace (`/my/employer/vacancies/applications`):
  every delivered application to the account's own vacancies, newest first, filterable by
  vacancy and stage, with the candidate's answers, note and the CV exactly as it was mailed.
- **A hiring pipeline** of six stages — new, reviewing, interview, offer, hired, rejected —
  the employer moves each application through.
- **The candidate's tracking follows the employer's move** (reviewing → screening, interview,
  offer, hired → accepted, rejected), recorded in the application-event ledger with a new
  `employer` source, never overriding a stage the candidate set or settled.
- **Interview, offer and rejection email the candidate**, optionally with the employer's own
  message, Reply-To the employer's confirmed work email.
- The CV sent with each application is kept, so the inbox can serve it later.

Not in this change (the seams are left open): several people per company (one account still
owns a company), interview scheduling, scorecards, a public reply-rate badge built on the
`employer` events.

## Capabilities

### New Capabilities
- `employer-applicant-pipeline`: the employer's inbox of on-site applications, the stages
  they move them through, and what the candidate sees and receives when they do.

## Impact

- Migration `0270`: `direct_applications.stage`/`stage_changed_at`, table
  `direct_application_cvs`.
- `internal/ingest/directapply` (`Inbox`), `internal/application/jobtracking`
  (`EmployerStages`), `internal/application/userjob` (`EmployerMay`),
  `internal/application/appevent` (`SourceEmployer`), `internal/engage/emailnotify`
  (`SendStatusUpdate`).
- Three cookie-only routes under `/api/v1/employer/applications` in `typespec/employer.tsp`.
- The SPA's employer dashboard gains the Applications tab.
