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
- **A suggestion needs a consumer, or the matcher's caution turns into a backlog.**
  Only a deterministic tier auto-links, so everything else lands as a suggestion —
  and a suggestion nobody can see is a row that never resolves. `?link=suggested`
  is that queue, `?link=unlinked` is the mail with no application to attach to,
  and `POST /me/emails/:id/application` is the way out of the second: it records
  the application from the mail and links it in one call. Keep all three reachable;
  measured link coverage is a function of the interface, not only of `mailmatch`.
- **An application recorded from mail is dated by the mail.** `received_at`, never
  `now()` — the application demonstrably existed by the time the employer wrote, so
  the mail's timestamp is an honest upper bound. The error then leans toward
  *under*-reporting elapsed silence, which is the safe direction: a missed ghost is
  a non-event, a fabricated one tells a person they were ignored when they were not.
- **Bodies reach an agent through the listing (`?body=1`), not `GET /me/emails/:id`.** That
  endpoint marks the message read, and `read_at` means "a human saw this" — a harness
  sweeping the backlog through it would silently zero its owner's unread count.
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
