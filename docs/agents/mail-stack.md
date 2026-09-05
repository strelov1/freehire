# Mail stack

Eight packages and three workers turn inbound recruiter mail into linked, stage-advancing
applications. Each package has a package doc explaining *what* it does; this file explains
how they compose and where the traps are.

## The pipeline

```
                 ┌── Gmail OAuth ──→ internal/application/gmailsync ──┐
inbound mail ────┤                                        ├──→ emails table
                 └── SES→S3→SQS ──→ internal/application/mailingest ──┘    (source = gmail|hosted)
                                                                      │
                                          email_classification_outbox │  ← excludes 'external'
                                                                      ▼
                                                        internal/application/maillink (runner)
                                                          ├─ internal/application/mailmatch   deterministic
                                                          └─ internal/application/mailclassify  LLM + vocabulary
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
| `internal/application/inbox` | The mail use cases (read, classify, link, record an application). Both readers call it |
| `internal/application/gmailsync` | Incremental Gmail OAuth connect + per-user sync worker (`cmd/gmail-sync`) |
| `internal/application/mailingest` | SES inbound: parse MIME → resolve recipient → store (`cmd/mail-ingest`, a daemon) |
| `internal/application/mailbox` | Derives `<handle>@<MAILBOX_DOMAIN>`; pure — allocation lives in the handler |
| `internal/application/maillink` | Drains the classification outbox, decides link + stage (`cmd/classify-mail`) |
| `internal/application/mailmatch` | Deterministic match: thread continuity, company name in sender/subject |
| `internal/application/mailclassify` | Status vocabulary, sanitizer, LLM adapter |
| `internal/application/mailrecall` | Pull direction: one Gmail search scoped to an employer, proposals only — never stored, never linked |

## Always true

- **The fetch is scoped to hiring-SHAPED mail, not to ATS senders, and the inbox filters at
  DISPLAY.** `gmailsync.BuildQueryFor` admits mail from a curated ATS domain OR carrying one
  of the recognised application/interview phrasings, minus anything the connected address
  itself sent. Two things about it are measured rather than assumed.
  *The phrase list's shape was the defect.* It held one canonical wording per idea, so over
  120 days it fetched 431 messages from a mailbox where a hiring-shaped query found 1151 —
  and the 739 misses were NEAR misses: an acknowledgement reading "we've received your …
  application" where the list knew only "your application at", an invitation reading
  "interview invite" where it knew only "invite you to interview". When adding, add the
  SIBLINGS of a wording, not the wording.
  *The junk filter is the classifier, and it runs at display.* Roughly 40% of any widening is
  aggregator and course marketing, and the answer is NOT a blocklist of domains: that is a
  second judge, curated by hand forever against people whose business is registering domains,
  judging by sender where `mailclassify` already judges by content on a call we already pay
  for. The listing therefore omits `status_signal = 'other'` by default and reports how many
  it omitted, under the SAME filters — a hidden count computed separately would describe a
  different mailbox the moment any filter is on. Two rules keep it honest: unclassified mail
  is NEVER hidden (nothing has judged it), and asking FOR the label overrides the default
  (`inbox.Query.showsOther`), or `?status=other` and the hide rule cancel and the filter
  answers nothing about its own subject.
  **Why display and not fetch:** the sync carries a watermark, so mail not fetched today is
  never fetched. A fetch-time filter loses silently and permanently, and fixing the rule
  brings nothing back. A display filter loses nothing — and it became affordable only once
  the recall sweep stopped reading our copy (it searches Gmail now), so extra rows degrade a
  list a person reads and nothing else.
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
- **`MAILBOX_DOMAIN` ≠ `NOTIFY_EMAIL_FROM`.** The first is the *receiving* domain (mailbox
  addresses; read by both the API's `mailboxReady` gate and `cmd/mail-ingest`); the second is
  the SES *sending* identity for notifications. They may point at the same host — they are
  still separate settings with separate IAM.
- Recipient lookup in `mailingest` is **case-insensitive**; addresses are allocated
  lowercase.
- **`mailbox.reservedHandles` blocks more than RFC 2142 role names.** It also reserves the
  CA/Browser Forum "constructed email addresses" (`admin`, `administrator`, `webmaster`,
  `hostmaster`, `postmaster`): domain validation lets a certificate be issued to whoever
  answers one at the domain, so allocating `admin@` would hand out the ability to get a TLS
  certificate for the receiving domain.
- **`EnqueuePendingEmailClassification` must keep excluding `source = 'external'`.** That
  one predicate is what makes the bring-your-own-harness tier free; drop it and every
  pushed message gets picked up by `cmd/classify-mail` on the next cron run and billed to
  us. Such mail stays `classified_at IS NULL` forever by design — `?unclassified=1` on the
  listing is how the harness finds its own backlog.
- **`UpsertExternalEmail` refreshes content columns only.** `read_at`, `deleted_at` and
  every classification column are the *reader's* state, not the mail server's, so a nightly
  re-sync cannot resurrect deleted mail, un-read a message, or wipe a triage verdict.
- **There is a PULL direction, and it SEARCHES the mailbox rather than reading our copy.**
  Everything above starts from a message and asks which application it belongs to.
  `POST /me/tracking/:slug/mail-recall` (`internal/application/mailrecall`) asks the opposite. It issues
  ONE Gmail search scoped to the employer inside a window around `applied_at`, adjudicates
  what comes back in one model call, and shows the confident answers.
  The first version read the stored table instead, and production retired it: candidates in
  that window ran 96 at minimum and 158 at the median, **263 of 263 applications exceeded
  the cap of 40**, and the batch was chosen oldest-first — which says nothing about
  relevance. The store simply cannot answer "about this employer": matched against sender
  name and subject the employer's name is absent from the median application's mail
  entirely. A mailbox search reads BODIES, and found mail for **14 applications in 15**
  where the store found none. A caller with no searchable mailbox still gets the stored
  path, which is why both still exist.
  Four rules carry it:
  *The search is gated* — employer AND (hiring vocabulary OR `filename:ics` OR the role
  title) — because scoping by a company name would otherwise reach personal mail. All three
  gate members were measured: hiring words alone cut 53 candidates to 41 and dropped both
  calendar invitations for one interview plus a live thread whose only subject was the role.
  `filename:ics` is the exact member for invitations, whatever language the subject uses.
  *A proposal is not stored.* The sweep keeps nothing; pressing Link imports the message
  (idempotent on `(source, external_id)`) and then links it. What nobody confirmed is not
  kept.
  *The model is addressed by POSITION in the batch, never by an id* — a searched message
  has none of ours, and an out-of-range position resolves to nothing by construction.
  *It never links, never advances a stage, never writes the ledger* — held by
  `TestMailRecallCannotLink`, which now walks both `Store` and `Mailbox`, and by a source
  scan of the package for `LinkEmailToJob` / `application_events` / `calsync`.
  **The calendar still needs no code**: `cmd/cal-sync` re-reads its whole ±90-day window
  every run, so an invitation linked here yields its meeting on the next one.
- **`internal/application/calsync` stores only meetings an application earned.** `cmd/cal-sync` reads each
  connected candidate's calendar ±90 days around now and writes ONLY a meeting `calmatch` can
  attach to one of the candidate's own applications — the schema backstops the rule
  (`application_interviews.application_id` is NOT NULL), so a mistake cannot become a stored
  dentist appointment. The whole window is re-read on every run; there is no incremental
  state to drift. The only status ever written is `confirmed`. Connections are filtered by
  the scopes recorded at consent, so a Google grant that predates the calendar consent never
  reaches the worker and never costs an API call to discover. The worker shares the Gmail
  Google grant and `gmailsync.CalendarScope`, and nothing else.
- **Two known gaps in the PUSH path, measured and not yet fixed.** They are why the pull
  direction has so much to find. `gmailsync.BuildQuery` fetched **431** messages over 120
  days from a mailbox holding **3297**; **739** hiring-shaped messages were never fetched,
  including an acknowledgement, three interview invitations and four live recruiter threads
  from personal and corporate domains. The misses are near misses on wording — the phrase
  list knows `invite you to interview` but not `interview invite`. Separately,
  `mailmatch.ExtractCompany`'s five subject templates all use `to`, while the mailbox shows
  `at`, `with`, `as … at`, a company before a dash, a company after a pipe, and Portuguese;
  33 of 93 messages with an empty sender name carry a subject it cannot parse.
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
  tools (`chat` preset only) call `internal/application/inbox` directly with the session owner's id.
  That package exists because of it: a rule enforced in a Fiber handler is a rule the
  in-process agent never meets, which is exactly how the CV-tailoring contact guard was
  lost. Put a new mail rule in the service, not in a handler, and check `IsNotFound` /
  `InvalidError` render sensibly for both readers. The assistant's own bounds — no
  single-message read, a 10-message body cap, no tool that sends — are in
  `internal/ai/assistant/AGENTS.md`.
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
- `internal/application/gmailsync/learn.go` self-learns confident job-mail sender domains; promotion is
  count-based and has no decay.
