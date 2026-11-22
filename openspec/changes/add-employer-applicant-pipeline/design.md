## Context

`direct_applications` (migrations 0187, 0190) already holds every on-site application, one
row per (job, candidate), stamped `sent_at` once the email reached the employer. The
`add-in-app-apply` design named the employer inbox as its seam. The candidate's side has a
stage vocabulary (`userjob.Stages`), a state machine for automatic moves (`userjob.Forward`)
and an event ledger (`application_events`) whose sources say who observed a change.

## Goals / Non-Goals

**Goals:** the employer sees, opens and decides on every application to their vacancies on
freehire; the candidate sees each decision without the employer writing a letter.

**Non-Goals:** company teams (one account per company stays), scheduling, scorecards, offers
as documents, a public responsiveness metric (the `employer` events are its seam).

## Decisions

### 1. The pipeline is a column on `direct_applications`, not a history table
`stage` (CHECK over the six names, default `new`) and `stage_changed_at`. The history a
reader may want — when the employer first answered — is already written: each move the
candidate can see lands in `application_events` with source `employer`, timestamped as the
employer acts. A second history table would be a second record of the same transitions.

### 2. The CV is kept as a copy, in Postgres
The inbox must show the CV the employer was sent. Re-rendering the candidate's tailored CV on
demand would show its current state, or nothing once deleted, and the CV row is `SET NULL`
on delete precisely so a deletion does not un-send an application. So the file each attempt
mails is written to `direct_application_cvs` in the same statement that records the attempt
(a data-modifying CTE in `UpsertDirectApplication`); a retry replaces it. Postgres rather than
the S3 blob store because the row then cascades with the application — from the candidate's
account deletion and from the job — with no second cleanup path, and volume is bounded (one
CV per application to an on-site vacancy, capped per candidate per day). Moving to the blob
store is a change of this one table if volume ever says so. Applications sent before 0270
have no copy and say so (`has_cv: false`).

### 3. Ownership is the vacancy's, through one predicate
Every inbox statement joins the application to its job and requires
`created_by = employer AND source = 'employer' AND sent_at IS NOT NULL` — the same ownership
predicate as the employer's vacancy writes. Company teams will change that predicate in one
place. An unsent application is invisible: the employer was never sent it.

### 4. The candidate's tracking follows, under a rule of its own
`userjob.EmployerMay(current, currentByEmployer, target)`: unlike `Forward` (mail inference,
never into or out of a settled stage), an employer's move is an explicit statement, so it may
settle an application. It still never overrides the candidate: a stage the candidate set
moves only forward, one they settled (withdrawn, or an outcome they recorded) is never
reopened, and an application they took off their board is not recreated. A stage the employer
set is theirs to correct in any direction — a rejection clicked by mistake must be undoable.
Whose stage it is reads from the newest live `stage_set` event's source. The write is a
compare-and-set against the stage just read (`MirrorEmployerStage`), so a candidate's
concurrent change wins. `new` maps to `applied`, what the record reads on arrival, so a move
back to it undoes a stage the employer set by mistake. Whose stage it is reads from the most
recently written event (by id), not the newest `occurred_at`: mail events carry the message's
own date, which can be older than an employer move recorded before them. A record the
candidate cleared (stage and applied date both NULL) counts as no record.

`SourceEmployer` is trusted for day math, like mail and calendar: the timestamp is set by the
employer acting, on our clock, not by the candidate getting round to recording it.

### 5. Which moves email the candidate
Interview, offer and rejected — the moves a candidate acts on. Reviewing shows in tracking
without a letter; a hire was agreed with the candidate before the employer records it. The
letter is `GroupEssential` with no unsubscribe, like the application it answers: it is the
employer's reply to a letter the candidate sent. Reply-To is the account's work email, the
symmetric counterpart of the application's Reply-To the candidate. An optional message (≤
2000 characters) is quoted in it; a message with a stage that sends no letter is refused, not
dropped. The move is saved before the letter, and a failed letter is reported as
`candidate_notified: false` and logged — never reported as sent, never failing the move. A
repeated move to the same stage is a no-op (the row is locked, `changed` reported) and
notifies nobody.

### 6. Where it lives
`directapply.Inbox`, beside the candidate-side `Service` in `internal/ingest/directapply`:
it needs the employer account (`employer.Service.ActiveAccount`) like `Service` does, and the
candidate's tracking (`application`, a lower layer). Mail is a port implemented in
`emailnotify` (`ApplicationMailer.SendStatusUpdate`) and wired in `handler.Register`, as for
the application mail. The inbox is built whenever mail is configured — it renders nothing, so
it needs no Typst — and its routes answer 501 otherwise.

### 7. The SPA puts it under Vacancies, as a routed tab
`/my/employer/vacancies/applications`, not a third sidebar item: one more nav href pushes the
account-nav href union past what SvelteKit's `resolve()` can relate (`accountNav.ts`).

### 8. One move of an application at a time, the letter after the commit
Two overlapping moves of one application could otherwise run their follow-ups in reverse —
an older interview move mirroring and mailing after a newer rejection — and a move queued
behind another could compare against a stale stage. `ApplicationMoves` runs each move on
one pooled connection holding the application's session advisory lock: the stage write
(one `UPDATE` comparing against the row as it reaches it) and the candidate's mirror (in a
savepoint, so a database failure there leaves the stage standing) commit in one
transaction on that connection, and only then the letter is sent, still inside the lock.
One connection per move keeps concurrent moves from exhausting the pool; a letter is only
ever about a committed decision; letters keep the order of the moves. The lock is released
even after the request's context ends, and the connection is closed if releasing fails.

## Risks / Trade-offs

- **Bytea growth.** Bounded by the daily cap and by on-site vacancies only; the table is
  separate so list queries never read it.
- **Mis-click emails.** A rejection sent by mistake has reached the candidate; the employer
  can move it back (their own stage), which corrects the tracking but not the letter.

## Inbox search and private workspace

Search compares case-insensitive literal text against the applicant email,
candidate message, and stored form answers and labels. It applies before the
page window. The page total matches all filters, and the counters continue to
describe every stage for the selected vacancy. The search text travels in a
POST body. Inbox and activity responses carry private, no-store caching.

The viewed timestamp records the team's first explicit review and is independent
of the hiring stage. The employer team shares one private note field per
application. Updating that field does not change applicant tracking or send
email. Stage changes retain the existing tracking and email behavior.

Migration 0296 adds the fields and activity log. Existing delivered applications
start with an empty note and no recorded review. Their stages and CV snapshots
are preserved. Applications from before CV archiving remain readable and display
that the original was available only by email.
