# Mail stack

Six packages and three workers turn inbound recruiter mail into linked, stage-advancing
applications. Each package has a package doc explaining *what* it does; this file explains
how they compose and where the traps are.

## The pipeline

```
                 ┌── Gmail OAuth ──→ internal/gmailsync ──┐
inbound mail ────┤                                        ├──→ emails table
                 └── SES→S3→SQS ──→ internal/mailingest ──┘    (source = gmail|hosted)
                                                                      │
                                          email_classification_outbox │  ← excludes 'external'
                                                                      ▼
                                                        internal/maillink (runner)
                                                          ├─ internal/mailmatch   deterministic
                                                          └─ internal/mailclassify  LLM + vocabulary
                                                                      │
                                                        link + monotonic stage advance

the caller's own harness ──→ POST /me/emails ──→ emails table (source = external)
                                                                      │
                                             the harness classifies it itself, then
                                             POST /me/emails/:id/triage ──→ same columns,
                                                        same stage-advance rules
```

There is a **third source with no worker behind it**. `external` is mail a user's own
client fetched and pushed; we provide no transport for it and never classify it. That is
the point: it is the tier that costs us nothing. See the `external` bullets below.

| Package | Role |
|---|---|
| `internal/inbox` | The mail use cases (read, classify, link, record an application). Both readers call it |
| `internal/gmailsync` | Incremental Gmail OAuth connect + per-user sync worker (`cmd/gmail-sync`) |
| `internal/mailingest` | SES inbound: parse MIME → resolve recipient → store (`cmd/mail-ingest`, a daemon) |
| `internal/mailbox` | Derives `<handle>@<MAILBOX_DOMAIN>`; pure — allocation lives in the handler |
| `internal/maillink` | Drains the classification outbox, decides link + stage (`cmd/classify-mail`) |
| `internal/mailmatch` | Deterministic match: thread continuity, company name in sender/subject |
| `internal/mailclassify` | Status vocabulary, sanitizer, LLM adapter |

## Always true

- **Never match on the sender-address domain.** Inbox mail arrives from ATS relay domains
  (`ashbyhq.com`, `greenhouse-mail.io`, …), not from employer domains. `mailmatch` matches on
  thread continuity and the company name carried in the sender *name* / subject. A
  domain-based shortcut looks obvious and is wrong.
- **The sender *name* is a relay too, and `atsPseudoNames` is the only thing saying so.**
  The rule above bans the domain but not the display name, and relays sign mail with their
  own brand: a message reading `From: Workable` / `Subject: Thanks for applying to Derq` is
  about Derq. Because `ExtractCompany` prefers the sender name over the subject, a brand
  missing from that list wins over the employer the subject names. `workable` was missing,
  and one catalog company called Workable auto-collected 23 acknowledgements belonging to 23
  other employers — an application that then could never look silent, while the real ones
  lost their mail. Adding a brand there is deliberately lossy (that brand's own mail stops
  auto-linking, degrading to a suggestion) and that trade is always worth taking.
- **Read the body via `readableBody(text, html)`, never `body_text` alone.** Many ATS senders
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
- **The suggestion is silenced by the ledger, not by a flag.** A `stage_set` in
  `application_events` later than the message means the candidate has already answered — whichever
  stage they chose. `LastStageSetAt` reads it. Do not add a dismissal column: it would be a second
  store of a decision the ledger already holds, and the two would drift.
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
- **`MAILBOX_DOMAIN` ≠ `MAIL_DOMAIN`.** The first is the *receiving* domain (mailbox
  addresses; read by both the API's `mailboxReady` gate and `cmd/mail-ingest`); the second is
  the SES *sending* identity for notifications. They may point at the same host — they are
  still separate settings with separate IAM.
- Recipient lookup in `mailingest` is **case-insensitive**; addresses are allocated
  lowercase.
- **`EnqueuePendingEmailClassification` must keep excluding `source = 'external'`.** That
  one predicate is what makes the bring-your-own-harness tier free; drop it and every
  pushed message gets picked up by `cmd/classify-mail` on the next cron run and billed to
  us. Such mail stays `classified_at IS NULL` forever by design — `?unclassified=1` on the
  listing is how the harness finds its own backlog.
- **`UpsertExternalEmail` refreshes content columns only.** `read_at`, `deleted_at` and
  every classification column are the *reader's* state, not the mail server's, so a nightly
  re-sync cannot resurrect deleted mail, un-read a message, or wipe a triage verdict.
- **There is a PULL direction, and it proposes rather than links.** Everything above starts
  from a message and asks which application it belongs to.
  `POST /me/tracking/:slug/mail-recall` (`internal/mailrecall`) asks the opposite, so an
  application that plainly ought to have mail can say so. It gathers the caller's
  unattached live mail in a window around `applied_at` — **oldest first**, capped at 40 —
  adjudicates the batch in ONE model call, and writes the confident answers to
  `suggested_job_id`. Four things make it safe, and each was nearly got wrong:
  *"unattached" needs BOTH `job_id IS NULL` and `application_id IS NULL`*, because a
  message auto-linked before its application row existed holds the first without the second
  and nothing repairs it but a one-shot backfill; *the net is state and time, never the
  employer's name*, which would reproduce `mailmatch`'s measured blind spot and additionally
  miss every HTML-only sender; *the cap eats from the far end*, because newest-first over an
  open window spends forty candidates on recent noise and never shows the model the
  acknowledgement; and *the guard is in the statement*, so a linked message is unreachable
  even if the net, the model and the service all went wrong at once. It never links, never
  advances a stage and never writes the ledger — `TestMailRecallCannotLink` and a source
  scan of the package hold that. **The calendar needs no code**: `cmd/cal-sync` re-reads its
  whole ±90-day window every run, so an invitation confirmed here yields its meeting on the
  next one.
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
- **A linked message counts as a reply whether or not it is classified.** Requiring a
  classification reads as the stricter rule and is the opposite: `external` mail is never
  classified server-side by design, so that tier's replies would never count and their
  employers would read as more silent than they were.
- **An application recorded from mail is dated by the mail.** `received_at`, never
  `now()` — the application demonstrably existed by the time the employer wrote, so
  the mail's timestamp is an honest upper bound. The error then leans toward
  *under*-reporting elapsed silence, which is the safe direction: a missed ghost is
  a non-event, a fabricated one tells a person they were ignored when they were not.
- **There is a third reader, and it issues no HTTP request.** The in-app assistant's mail
  tools (`chat` preset only) call `internal/inbox` directly with the session owner's id.
  That package exists because of it: a rule enforced in a Fiber handler is a rule the
  in-process agent never meets, which is exactly how the CV-tailoring contact guard was
  lost. Put a new mail rule in the service, not in a handler, and check `IsNotFound` /
  `InvalidError` render sensibly for both readers. The assistant's own bounds — no
  single-message read, a 10-message body cap, no tool that sends — are in
  `internal/assistant/AGENTS.md`.
- **Bodies reach an agent through the listing (`?body=1`), not `GET /me/emails/:id`.** That
  endpoint marks the message read, and `read_at` means "a human saw this" — a harness
  sweeping the backlog through it would silently zero its owner's unread count. For the
  in-process reader this is structural: `inbox.Queries` has no read-marking method, so
  `Search` cannot mark even by mistake.
- The whole `/me/gmail|inbox|emails|mailbox` surface is `mw.key` — a session cookie **or** a
  full-scope API key, so a user's own agent harness drives it. The lone exception is the
  Gmail OAuth connect/callback pair, which stays cookie-only: it redirects a browser to
  Google's consent screen and a keyed client cannot complete it.

## Infrastructure notes

The SES inbound pipeline is inherited from the retired `apply` app, so the AWS resources
still carry `freehire-apply-*` names (`freehire-apply-inbound-mail` queue,
`freehire-apply-mail-raw-*` bucket). **Do not rename them in terraform** — it would recreate
the queue and bucket. `cmd/mail-ingest` runs as a long-lived unit (`Restart=always`), unlike
every other worker in this repo.

## Limitations

- Gmail sync runs under an unverified restricted-scope OAuth app — test users only until
  Google verification.
- `internal/gmailsync/learn.go` self-learns confident job-mail sender domains; promotion is
  count-based and has no decay.
