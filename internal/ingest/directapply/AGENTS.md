# On-site applications

## Scope
`internal/ingest/directapply` — a candidate's application to an employer vacancy that takes
applications on freehire (`job.IsDirectApply`: source `employer`, external id
`freehire:<uuid>`, no URL). Checking it against the employer's questions, recording it, and
delivering it by email to the employer account's confirmed work address. The vacancy and its
questions are authored in [internal/ingest/employer](../employer/AGENTS.md); the design
record is [openspec/changes/add-in-app-apply](../../../openspec/changes/add-in-app-apply).

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
- **An answer is a snapshot.** Each stored answer carries its question's id, label and
  type, so a record still reads correctly after the employer edits the questions. A boolean
  is `"true"`/`"false"` on the wire and banked as `Yes`/`No`, because the bank also prefills
  ATS forms that ask in prose.
