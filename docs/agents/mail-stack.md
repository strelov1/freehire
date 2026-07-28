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
                                          email_classification_outbox │
                                                                      ▼
                                                        internal/maillink (runner)
                                                          ├─ internal/mailmatch   deterministic
                                                          └─ internal/mailclassify  LLM + vocabulary
                                                                      │
                                                        link + monotonic stage advance
```

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
- The whole `/me/gmail|inbox|emails|mailbox` surface is `RequireRole("moderator")` — exact
  match, admin is *not* admitted.

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
