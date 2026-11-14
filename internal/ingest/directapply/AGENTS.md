# On-site applications

## Scope
`internal/ingest/directapply` — a candidate's application to an employer vacancy that takes
applications on freehire (`job.IsDirectApply`: source `employer`, external id
`freehire:<uuid>`, no URL). Checking it against the employer's questions, recording it, and
delivering it by email to the employer account's confirmed work address (`Service`); and the
employer's side of it, the applicant inbox and hiring pipeline (`Inbox`, `inbox.go`). The
vacancy and its questions are authored in [internal/ingest/employer](../employer/AGENTS.md);
the design records are [openspec/changes/add-in-app-apply](../../../openspec/changes/add-in-app-apply)
and [openspec/changes/add-employer-applicant-pipeline](../../../openspec/changes/add-employer-applicant-pipeline).

## Always true
- **Recorded first, stamped sent after.** `Apply` writes the `direct_applications` row,
  sends the mail, and only then sets `sent_at`. A failed send returns `ErrDelivery` and
  leaves the row unsent; the candidate applying again REPLACES its content and retries. The
  reverse order would let a failed send look delivered — an application lost with nothing
  reporting it. The cost is that a crash between send and stamp sends twice on a retry,
  which is the better failure.
- **A delivery holds a claim.** `sending_at` is set when the row is recorded and cleared
  when it is stamped sent or the send fails; a second request meanwhile gets
  `ErrInProgress`. Without it a double click emails the employer twice, since both requests
  find the row unsent. A claim older than two minutes counts as abandoned.
- **One row per (job, candidate), enforced by the table.** `UpsertDirectApplication`
  refuses to touch a sent row, and the daily cap (`DIRECT_APPLY_DAILY_LIMIT`, default 30)
  counts OTHER vacancies only, so a retry never costs a second slot. When it writes
  nothing, `Sent` tells "already applied" from "cap reached".
- **Tracking and the answer bank follow delivery and never fail it.** They run only once
  the employer has the application, and their errors are logged, not returned — telling a
  candidate their delivered application failed would invite a duplicate.
- **This package sits in `ingest`, not `application`**, because it needs the employer
  account (`employer.Service.ActiveAccount`), which `application` (layer 6) cannot import.
  The question builder lives in `applyform`, not here: `employer` needs it and this package
  imports `employer`, so placing it here would be a cycle.
- **Mail and the CV PDF are ports.** `Mailer` is implemented in `internal/engage/emailnotify`
  (same layer, so not importable) and `CVs` by `internal/api/handler`, which already holds
  the renderer, the headshot store and the tracer-link minter. Both are wired in
  `handler.Register`; without Typst or SES the endpoint answers 501 rather than recording an
  application nobody can deliver.
- **Two CVs, never a pick-list.** An application sends the CV the candidate tailored for
  THAT vacancy (`cvs.job_id` must equal the vacancy — checked in the handler's
  `directApplyCVs`, not just offered by the form) or the file they uploaded to their profile
  (`resume.Store.File`, attached as uploaded: `cv.pdf`, or `cv.txt` for pasted text).
  `direct_applications.cv_kind` (migration 0190) records which, because a NULL `cv_id` alone
  could mean "the uploaded file" or "a tailored CV since deleted". A base CV or one tailored
  for another vacancy was once the first option offered — the wrong document to send.
- **An answer is a snapshot.** Each stored answer carries its question's id, label and
  type, so a record still reads correctly after the employer edits the questions. A boolean
  is `"true"`/`"false"` on the wire and banked as `Yes`/`No`, because the bank also prefills
  ATS forms that ask in prose.

## The applicant inbox (`Inbox`)
- **Reached only through the vacancy, and only once delivered.** Every inbox statement joins
  the application to its job and requires `created_by = employer AND source = 'employer'`
  plus `sent_at IS NOT NULL` — the ownership predicate the employer's vacancy writes use.
  Anything else, another employer's application included, is `ErrNotFound`. An unsent row is
  not the employer's to see: they were never sent it, and the candidate's retry may replace
  it. Every method also requires `ActiveAccount`, so a revoked account reads nothing.
- **The CV is a copy of what was mailed** (`direct_application_cvs`, migration 0270), written
  in the same statement as the application, so a retry replaces it with the file the retry
  mails. It is never re-rendered from the candidate's current CV: a tailored CV edited or
  deleted afterwards must not change what the employer was sent. It lives in Postgres, not
  the blob store, so it cascades with the application on account deletion. Applications sent
  before the table existed have no copy (`has_cv: false`) — their CV went by email only.
- **The stage is the employer's record and is saved first.** `Move` writes it, then the
  candidate's tracking follows (`StageMirror`; how far is `userjob.EmployerMay`'s call) and,
  for interview/offer/rejected, the candidate is emailed (`StatusNotifier`, Reply-To the
  account's work email). Neither follow-up fails the move; a failed letter is reported as
  `candidate_notified: false`, never as sent. A repeated move to the same stage changes
  nothing and notifies nobody (`SetDirectApplicationStage` is one UPDATE that compares
  against the row as the update reaches it, and reports `changed`). The whole move is one
  `ApplicationMoves` run on one pooled connection holding the application's session
  advisory lock (`LockDirectApplication`), so an older move can never mirror or mail after
  a newer one and a move never holds one connection while waiting for another. The stage
  and the mirror commit together in one transaction (the mirror in a savepoint, so its
  failure leaves the stage standing); the letter is the after-commit step, still inside the
  lock, so it is only ever about a saved decision. The lock is released even when the
  request's context has ended, and the connection is closed if releasing fails. A message is refused with a stage that sends no letter rather than dropped.
- **Every stage maps onto a userjob stage** (`candidateStage`): new → applied (what the
  record reads on arrival, so moving back undoes a stage the employer set by mistake),
  reviewing → screening, hired → accepted, the rest by name. Whether the candidate's record
  actually moves is `userjob.EmployerMay`'s call.
- **Built when mail is configured, not Typst.** The inbox serves the kept CV and renders
  nothing, but a move may email the candidate, so without a mail transport its routes answer
  501 — wired in `handler.Register` beside `Service`.
