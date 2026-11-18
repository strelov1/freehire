# Mail stack

Nine packages and three workers turn inbound recruiter mail into linked, stage-advancing
applications. Each package has a package doc explaining *what* it does; this file explains
how they compose and where the traps are.

## The pipeline

```text
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
| `internal/application/mailreply` | Drafts the candidate's reply to one received message (`POST /me/emails/:id/reply-draft`, paid, `mail-reply` allowance). Text only: never sent, never stored |

## Always true

- **Gmail keeps both directions inside hiring threads.** Incoming search excludes SENT and
  DRAFT mail; thread expansion retains sent replies and discards drafts. Gmail's SENT label
  identifies aliases; the connected primary address is a fallback. `emails.direction` is
  `incoming` or `outgoing`, with To/Cc headers and thread ids exposed to inbox readers.
  Sent messages start read and never enter employer classification, stage advancement,
  employer-reply events, interview evidence or silence calculations. Hosted and external
  messages retain their existing incoming defaults; this adds no sending transport.
  Application mail views also show unlinked sent replies when their Gmail conversation
  has one unambiguous linked incoming job, including links classified after sync. Explicit
  attachments win; this display inference writes no link or employer activity. Recall
  candidates and suggestion writes remain incoming-only, including after alias repair.
- **Sent discovery has its own cursor.** `gmail_connections.sent_sync_cursor` starts at zero:
  the first successful pass expands stored Gmail threads to recover their sent history.
  Later passes list SENT message/thread identifiers since the prior successful run (with a
  60-second overlap), fetching content only for known hiring threads. This catches replies
  to old conversations without a new incoming anchor. The cursor records run-start time;
  a transient message or thread failure preserves both cursors, and idempotent inserts
  make retries safe. Permanently missing provider messages/threads are skipped. Reimports
  refresh direction and recipient metadata; an old alias reply newly recognized as SENT
  loses its employer classification, pending work and employer-reply event. Existing
  application stages are not automatically rolled back: that would overwrite later user
  decisions. Apply migration 0267 before deploying the readers.
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
  (`inbox.Query.ShowsOther`), or `?status=other` and the hide rule cancel and the filter
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
- **Reading the body, the two confidence gates, the deterministic tiers, what a signal implies vs. applies, `stage_set`, the classifier prompt, and the classification worker (consumers, ledger reconcile, claims, ownership, time-bounded failures)** are in [mail-stack/linking.md](mail-stack/linking.md).
- **`MAILBOX_DOMAIN` ≠ `NOTIFY_EMAIL_FROM`.** The first is the *receiving* domain (mailbox
  addresses; read by both the API's `mailboxReady` gate and `cmd/mail-ingest`); the second is
  the SES *sending* identity for notifications. They may point at the same host — they are
  still separate settings with separate IAM.
- Recipient lookup in `mailingest` is **case-insensitive**; addresses are allocated
  lowercase.
- **The hosted recipient is `receipt.recipients`, never `mail.destination`.** SES publishes
  two recipient lists in the same notification and only one of them says where the message
  was delivered: `receipt.recipients` is the envelope RCPT TO that matched the receipt rule,
  while `mail.destination` is assembled from the To/Cc *headers* — AWS's own reference says
  in as many words that the two may differ. `source_ses.go` read the headers until
  freehire#3027, and that cost two different things. A **forward never arrived**: a Sieve
  `redirect :copy "<handle>@mail.freehire.me"` rewrites the envelope and leaves the headers
  naming the mailbox that forwarded, so the worker found no address on our domain and
  dropped a message SES had accepted, stored in S3 and marked `spf=pass dkim=pass
  dmarc=pass` (13 messages in the week it was measured). And the header list is **the
  sender's to write**, so resolving a mailbox from it let anyone deliver a message into
  another user's inbox by putting that user's address in `To:` while sending to their own.
  The guard is that `sesNotification` no longer decodes the field at all: what is not
  decoded cannot be read back by mistake. An envelope recipient the notification does not
  carry is dropped rather than guessed from the headers.
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
- **The pull direction, the calendar sync, connection revocation and the known push-path gaps** are in [mail-stack/pull-and-sync.md](mail-stack/pull-and-sync.md).
- **The self-learning ATS-domain cache is fed by Gmail mail only** — hits and misses alike.
  A miss demotes, so a forged `From` could push a real ATS out as easily as a hit could pull
  one in. What `learner.Learn`
  records is a domain read off `From`, and the cache decides which senders a future sync
  accepts. Gmail authenticated that `From`; hosted mail's is whatever SMTP claimed and
  external mail's is whatever the caller's harness pushed, so on those tiers anyone able to
  post to an address could nominate the domain they want trusted. (Whether the cache should
  be per-user at all is a separate, open question about what it is for.)
- **`cmd/mail-ingest` reads a bounded object, and refuses rather than truncates.** `Receive`
  holds up to ten raw MIME messages at once in the only `Restart=always` daemon on the host,
  and the S3 read is capped at SES's own 40 MiB inbound ceiling — so nothing the bound
  rejects was ever deliverable mail. A bare `LimitReader` would be worse than no bound at
  all: MIME headers are at the front, so a half-read message parses and would be stored as
  though it were whole. An oversize object is acked and dropped (the raw stays in S3), like
  an undecodable notification: it is the same size on every redelivery.
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
- **Mark-all-read carries every filter the listing does, and that is a hand-kept pairing.**
  Keeping the bulk write out of `inbox.Queries` is what makes `Search` unable to mark — and
  it also means `MarkAllEmailsRead`'s predicates are a second copy of `ListEmails`'s, with
  nothing compiling them together. Two went missing that way: `unclassified` and
  `include_other`, both parsed and validated by the handler, neither reaching the statement,
  so `read-all?unclassified=1` emptied the WHOLE unread mailbox instead of the triage queue
  in front of the person pressing it. There is no undo. `include_other` is fed
  `Query.ShowsOther()`, not the raw flag, or `read-all?status=other` marks nothing.
  `TestMarkAllReadHonoursEveryFilterTheListingDoes` walks the three cases.
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

- Gmail sync runs under an unverified restricted-scope OAuth app — test users only (100 of
  them, Google's cap on a project in Testing) until Google verification. Lifting that is a
  manual, console-only errand with a recurring bill attached, because `gmail.readonly` is
  restricted and therefore carries an annual paid security assessment; the calendar scopes
  beside it do not. [../google-oauth-verification.md](../google-oauth-verification.md) is
  the runbook, and holds the scope justifications and demo-video script the forms ask for.
- `internal/application/gmailsync/learn.go` self-learns confident job-mail sender domains. Promotion
  needs `PromoteThreshold` hits AND at most `MaxOtherShare` of the domain's mail classified
  `other` (misses, `learned_ats_domains.other_hits`, migration 0185): a promoted domain's
  whole mailstream joins every user's query, and a count alone had admitted google.com and
  linkedin.com. There is no decay; a demoted domain's misses stay.
