## Why

Today a signed-in user can only feed their ATS mail into freehire by **connecting
their own Gmail** (OAuth `gmail.readonly`). That path is real but gated by Google's
restricted-scope verification — until the app is verified, only manually-added test
users can connect, so it cannot be offered to everyone.

We want a **second, self-contained option that works for every user with no Google
dependency**: give each user a **mailbox on our own domain** (`<handle>@inbox.freehire.dev`)
that they can use as their apply address (or forward ATS mail to). Mail sent there is
received by us, parsed, and shown in the **same inbox** as their Gmail mail. The two
sources sit side by side ("two options") and land in one unified inbox.

## What Changes

- **A hosted mailbox per user.** A user can claim an address on our receiving
  subdomain. Inbound mail is received via **AWS SES** (MX → SES receipt rule → raw
  MIME in S3 → SNS→SQS), drained by a run-as-daemon worker that parses the MIME,
  resolves the recipient to the owning user, and stores the message. Best-effort and
  idempotent by RFC `Message-ID`, mirroring the freehire-apply mail ingest.
- **A unified mail store.** The existing Gmail-specific `emails` table is
  **refactored into a source-agnostic message store** (`source` = `gmail` | `hosted`,
  a shared `external_id`, plus hosted-only `s3_key`). Both the Gmail sync worker and
  the SES ingest worker write through it; the inbox reads one list regardless of source.
  This keeps the two options symmetric instead of bolting a parallel table beside Gmail.
- **Read/unread state.** Messages gain a `read_at` stamp (absent today); opening a
  message marks it read, and the inbox reflects it. Applies to both sources.
- **Inbox surfaces the hosted option.** The `/my/inbox` page gains a "Get a freehire
  mailbox" action beside "Connect Gmail", shows the claimed address, and lists mail
  from both sources together.

NON-GOALS (this change): classification/labelling of mail; matching mail to catalogue
jobs or advancing tracker stages; **sending** applications from the hosted address
(the full freehire-apply loop); multiple mailboxes per user; non-SES inbound transports.

## Capabilities

### New Capabilities

- `hosted-mailbox`: claim/allocate a per-user address on our receiving subdomain
  (`<handle>@<MAIL_DOMAIN>`, collision-suffixed), the SES-backed inbound ingest worker
  that stores received mail idempotently under the owning user, and disconnect/release.
  Gmail-independent — works for every user.

### Modified Capabilities

- `email-inbox`: the inbox becomes **source-agnostic** — it lists a user's mail from
  Gmail **and** the hosted mailbox in one unified listing, and gains **read/unread**
  state (opening a message marks it read). The subject-grouped view is retained.

## Impact

- **New schema (migrations/):** `mailboxes` (per-user address); a **refactor migration**
  that generalizes `emails` → a source-agnostic store (`source`, `external_id`
  renamed from `gmail_msg_id`, nullable `s3_key`, `read_at`) and widens its uniqueness
  to `(user_id, source, external_id)`. `0014` is already live on prod, so the refactor
  is an in-place `ALTER` that preserves existing Gmail rows. *Migration-ordering caveat
  applies — apply before deploy (no versioned runner).*
- **New code:** `internal/mailbox` (address derivation + allocation), a SES inbound
  ingest package + `cmd/mail-ingest` daemon (parse MIME, resolve recipient, store),
  mailbox handlers + sqlc queries, unified inbox queries. Refactor of `internal/gmailsync`
  storage (`dbstore`/queries) to write through the unified store.
- **Refactored:** the Gmail store adapter and inbox handlers now speak the unified
  message shape; the Gmail sync behavior is unchanged at the spec level.
- **New env:** `MAIL_DOMAIN` (receiving subdomain), `AWS_REGION`, `MAIL_INBOUND_QUEUE_URL`,
  `MAIL_INBOUND_BUCKET` (SES→S3→SQS wiring). AWS creds come from the default chain
  (instance/SSO role), never app config.
- **New infra (freehire-ops / AWS):** a receiving subdomain with an MX record pointing
  at SES inbound; an SES receipt rule set → S3 bucket + SNS→SQS; an IAM role granting the
  worker S3-read + SQS-consume; a systemd service for the ingest daemon on host-2. SES
  inbound is region-restricted and needs the domain verified for receiving. **This infra
  is the critical path — the code is testable against a fake source without it, but the
  mailbox only receives mail once it is in place.**
- **Dependencies:** AWS SDK for Go v2 (`config`, `s3`, `sqs`) for the SES inbound source.
