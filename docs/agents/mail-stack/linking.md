# Mail stack: linking, staging and the classify worker

Part of the [mail stack](../mail-stack.md) conventions, the half of its "Always true" list about deciding what a message means: reading the body, the confidence gates and tiers, what a signal may apply, the ledger, and how the classification worker claims, links and fails.

- **Read the body via `maillink.ReadableBody(text, html)`, never `body_text` alone.** Many ATS senders
  (Gem, Ashby, Greenhouse) send **HTML-only** mail with no `text/plain` part, so `body_text`
  is empty and a classifier fed only that judges from the subject line — which once turned a
  plain rejection into `screening`. Bounded downstream by `TruncateRunes(4000)`.
- **Two confidence gates, two different decisions** (`maillink/decide.go`): `autoLink`
  decides auto-link vs. a suggestion the user confirms; `stage` decides automatic
  advancement. Only a *linked* email, at or above the stage threshold, whose signal maps
  **strictly forward**, advances a stage — never backward, never sideways.
- **Only a deterministic tier (`TierThread` / `TierName`) can auto-link.** A confident LLM
  pick becomes a *suggestion*, never a link. Keep that asymmetry: the LLM reads untrusted
  text.
- **What a signal IMPLIES and what it may APPLY are two fields, not one.** `mailclassify.StageFor`
  returns both, and `rejection → {rejected, advances:false}` is why: the message plainly means
  the application is over, and settling one stays the candidate's call. The table previously
  encoded "never automatic" by simply omitting rejection, which also made the meaning unsayable —
  so an application could hold seven messages including a plain rejection, sit at `interview`,
  and say nothing anywhere about why nothing moved. The Emails tab now states the implication per
  message, and `jobtracking.SuggestStage` offers the change in one press. It applies nothing by
  itself; `AdvanceStage` is unchanged and still reads only `advances:true`.
- **An automatic advance writes `stage_set` beside the column, in one statement.**
  `AdvanceUserJobStage` was a bare `UPDATE` until 2026-09-06, which made the mail path the one
  stage-writer that moved an application and recorded nothing — and the ledger, not the column,
  is what the aggregates read. `ListInterviewPrepCandidates` selects `kind='stage_set' AND
  signal='interview'` and nothing else, so `cmd/nudge` found no interview an employer's own
  invitation had produced; and since `SuggestStage` returns nil once the stage already matches
  what the mail implies, there was nothing to click either. **`occurred_at` is the message's own
  `received_at`**, read inside the statement, for the reason `RecordEmailApplicationEvent` gives:
  `now()` would compress a year of imported mail into the day a mailbox was connected.
- **The suggestion is silenced by the ledger, not by a flag.** A `stage_set` in
  `application_events` later than the message means the candidate has already answered — whichever
  stage they chose. `LastStageSetAt` reads it, **excluding the mail sources** (`appevent.MailSources`):
  now that mail records its own auto-advances there, a mail-derived event silencing a suggestion
  computed from the same messages would answer the question on the candidate's behalf. `system`
  stays in — an auto-expire is a decision about the application, not a reading of a message. Do
  not add a dismissal column: it would be a second store of a decision the ledger already holds,
  and the two would drift.
- **The classifier prompt carries the same three lessons the matcher learned.** The
  sender display name is usually the ATS, not the employer; a calendar invite the
  candidate organised themselves is `other`, not `interview_invitation`; and an
  employer absent from the candidate list means `matched_job_id = 0`, because an
  unlinked classification is useful while a wrong link transplants one employer's
  history onto another. `prompt_test.go` pins the prompt and `validSignals` to each
  other in both directions — a signal valid but undescribed can never be produced,
  and one described but invalid is coerced to `other`, so the model is asked for an
  answer that is thrown away.
- **A corroboration guard was measured and rejected.** Dropping any LLM pick whose
  company is not named in the message looks obviously right and would have caught
  the three mislinks that prompted this. Measured against 99 confirmed-correct links
  on a live mailbox, it would also have dropped **16** of them — recruiters routinely
  write without naming the employer. Prompt guidance carries this, not a hard rule.
- **`mailclassify` is the prompt-injection and out-of-vocabulary guard.** Email bodies are
  attacker-controlled. An unknown signal is sanitized to `other` before anything is
  persisted or served — do not persist a raw model string.
- **A suggestion needs a consumer, or the matcher's caution turns into a backlog.**
  Only a deterministic tier auto-links, so everything else lands as a suggestion —
  and a suggestion nobody can see is a row that never resolves. `?link=suggested`
  is that queue, `?link=unlinked` is the mail with no application to attach to,
  and `POST /me/emails/:id/application` is the way out of the second: it records
  the application from the mail and links it in one call. Keep all three reachable;
  measured link coverage is a function of the interface, not only of `mailmatch`.
- **Every link mutation ends with a ledger reconcile, and retraction is not deletion.**
  A linked message records an `employer_reply` in `application_events`, and that ledger —
  not this table — is what the per-company response rate reads. Five paths change the
  pairing (`SetEmailClassification`, `AgentTriageEmail`, `ConfirmEmailLink`,
  `LinkEmailToJob`, `UnlinkEmail`), and the classification worker is a sixth. The rule is
  one reconcile — `inbox.ReconcileMailEvent` — and it now really is one: the worker used to
  carry its own copy of the same three steps, inside a `cmd/` main where no domain test
  could reach it, under a comment asserting the rule had one home. The callers differ only
  in what they do with the error: the inbox's paths are best-effort (the mutation the user
  asked for already succeeded, and the reconcile is idempotent), while the worker propagates
  it to roll back the transaction that persisted the link. **Deleting a message changes nothing** —
  it hides content, it does not un-happen the reply — while **re-linking retracts and
  re-records**, because a wrong link left standing poisons a named company's public rate
  permanently (the Workable case above). The reconcile is deliberately **two statements in
  order**: written as one with data-modifying CTEs, every CTE reads the same pre-statement
  snapshot, so the insert's `ON CONFLICT` still sees the row the retract just stamped and
  silently records nothing.
- **The store rides with the claim, and dropping it stops the whole queue.**
  `ReconcileMailEvent` needs the mailbox that observed the reply, and `appevent.SourceForMail`
  refuses an empty one by design — so an unset `Claimed.Source` fails the transaction that
  persisted the link, and the message dead-letters after three attempts. The field was threaded
  through the query, the port and the ledger in one change; the only place that never took it
  was the hand-written copy in `cmd/classify-mail/store.go`, where Go's zero value made the
  omission legal and invisible. Every message queued between 2026-07-31 and 2026-09-05 died
  that way (2726 of them, across every user), and nothing said so: dead entries are never
  re-claimed, so each subsequent run truthfully logged `done failed=0 dead-lettered=0` and the
  binary kept exiting 0. `store_test.go` now walks `maillink.Claimed` by reflection, so a field
  added there and not mapped fails a test rather than the queue. **Recovery is not automatic** —
  `EnqueuePendingEmailClassification` is `ON CONFLICT DO NOTHING`, so a dead row stays dead
  until `failed_at`/`attempts`/`claimed_at` are cleared by hand.
- **The worker owns the classification, never a link someone else made.** A hand-made link
  does not stamp `classified_at` (neither `LinkEmailToJob`, `ConfirmEmailLink` nor
  `RejectEmailLink` does) and `EnqueuePendingEmailClassification` selects on
  `classified_at IS NULL`, so every manually linked message comes back round on the next
  five-minute tick. `SetEmailClassification` therefore keeps `job_id` / `application_id` /
  `link_source` / `match_confidence` whenever the row already reads `manual` or `agent` —
  the same precaution `AgentTriageEmail` carries from the other side. Without it the worker
  re-derived the link from a cascade that usually cannot reproduce it
  (`ListUserApplicationsForMatch` excludes closed postings, hosted mail carries no
  `thread_id`), so the common outcome was `NULL` — and since `ReconcileMailEvent` runs in the
  same transaction, the `employer_reply` event was retracted with it and the company started
  reading as silent in the reply-rate rollup. It still rewrites its OWN (`auto`) link: a
  later run correcting an earlier guess is the queue working.
- **A failure the message did not cause is bounded by TIME, not by attempts.**
  `maillink.messageAtFault` decides which of `FailEmailClassification`'s two branches
  applies, modelled on `enrich.postingAtFault` and defaulting to "not the message's fault".
  Only an undecodable model answer (`mailclassify.ErrUnparseableResponse`) spends the
  attempt ceiling; Postgres, the gateway, a schema-build failure and an unrecognised
  `emails.source` are all held to a 14-day queue-age window instead. The distinction is
  load-bearing because the lease makes a claimed entry re-claimable minutes later, so three
  attempts are spent well inside a quarter-hour outage — and, as the bullet above says,
  nothing here ever clears `failed_at`. An unrecognised source is deliberately on the
  *upstream* side: the one time that fired, the column was fine and the store's own mapping
  had dropped the field.
