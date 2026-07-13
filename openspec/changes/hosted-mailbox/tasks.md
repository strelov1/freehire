## 1. Unified mail store (schema + sqlc)

- [x] 1.1 Migration `0015`: create `mailboxes (id, user_id UNIQUE, address UNIQUE, created_at)`; refactor `emails` — `ADD COLUMN source TEXT NOT NULL DEFAULT 'gmail'`, rename `gmail_msg_id → external_id`, `ADD COLUMN s3_key TEXT`, `ADD COLUMN read_at TIMESTAMPTZ`; drop `UNIQUE (user_id, gmail_msg_id)`, add `UNIQUE (user_id, source, external_id)`; keep the received/subject indexes
- [x] 1.2 Rework `internal/db/queries/gmail.sql`: `UpsertEmail` writes `source='gmail'` via `external_id`; listing/group/get queries select `source` + `read_at IS NOT NULL AS read` and accept an **optional `source` filter** (empty = all accounts); add `MarkEmailRead(id, user_id)`; split mailbox queries into `internal/db/queries/mailbox.sql` (`GetMailboxByUser`, `GetMailboxByAddress`, `AllocateMailbox`, `DeleteMailbox`, `InsertHostedMessage`, `DeleteHostedMessagesByUser`); regenerate `internal/db`

## 2. Pure helpers

- [x] 2.1 `internal/mailbox`: `Handle(email)` (local-part → `[a-z0-9.-]`, lowercased, fallback) + `candidate(base, n)` collision suffix — pure, table-tested (port from apply)
- [x] 2.2 MIME parse for inbound: headers (from/subject/message-id/date), text + HTML bodies, best-effort — pure, table-tested (port/adapt apply `ingest/parse.go`)

## 3. Mailbox allocation + status API

- [x] 3.1 Allocation service: claim-or-return within a transaction, retrying the suffix on the `address` unique violation; unit-tested with a fake store
- [x] 3.2 `GET /me/mailbox` (address or null + `available`), `POST /me/mailbox` (claim), `DELETE /me/mailbox` (release + purge hosted mail); feature-gated on `MAIL_DOMAIN` (routes report unavailable when unset)
- [x] 3.3 Handler tests (integration) for claim idempotency, status, release-purges-hosted-only

## 4. SES inbound ingest

- [x] 4.1 `InboundSource` interface (`Receive`/`Ack`) + a fake for tests; ingest worker: parse → resolve recipient to mailbox/user → upsert `hosted` message (idempotent by Message-ID, synth key when absent) → ack; drop unknown-recipient/unparseable, leave transient store errors un-acked; unit-tested over the fake (port from apply `ingest/worker.go`)
- [x] 4.2 `SESSource` adapter: SQS long-poll → fetch S3 object → yield raw MIME; Ack deletes the SQS message; AWS config via default chain (add AWS SDK v2 deps)
- [x] 4.3 `cmd/mail-ingest` daemon: wire config (`MAIL_DOMAIN`, `AWS_REGION`, `MAIL_INBOUND_QUEUE_URL`, `MAIL_INBOUND_BUCKET`), run the poll loop, graceful shutdown; gated on config
- [x] 4.4 `internal/config`: add the mail-ingest env; document it in `.env.example`

## 5. Refactor Gmail store onto the unified shape

- [x] 5.1 Update `internal/gmailsync/dbstore.go` + `worker.go` to write through `external_id`/`source='gmail'` and the regenerated queries; keep the sync behavior identical
- [x] 5.2 Update inbox handlers (`internal/handler/inbox.go`) to the source-agnostic queries and the message wire shape (`source`, `read`); accept a `source` query param (validate against the known sources, empty = all); `GetEmail` marks read; Gmail deep-link stays Gmail-only
- [x] 5.3 Re-run existing gmail + inbox tests; adjust fixtures for the new columns

## 6. SPA (web/)

- [x] 6.1 `/my/inbox`: offer both actions ("Connect Gmail" + "Get a freehire mailbox"), show the claimed address, render the grouped list with unread styling, and an **account switcher** (`All` · `Gmail` · `freehire mailbox`) that sets the `source` filter — shown only for connected sources
- [x] 6.2 Mailbox claim/release wired to the new endpoints; copy-to-clipboard for the address
- [x] 6.3 `web/src/lib/api.ts`: mailbox types + methods; message shape gains `source`/`read`

## 7. Config + infra docs

- [x] 7.1 Document the AWS SES inbound setup (receiving subdomain + MX, receipt rule set → S3 + SNS→SQS, IAM role, region) as an ops checklist in the change/docs; note the systemd service for `cmd/mail-ingest`
- [x] 7.2 Note the `0015` migration must be applied to prod by hand before deploy (live `emails` table)

## 8. Verify

- [x] 8.1 `go build ./... && go vet ./...`, `go test ./...`, integration tests, `web` check pass
- [x] 8.2 End-to-end over the fake InboundSource: a synthesized inbound message resolves to a mailbox and appears in `/my/inbox` alongside Gmail mail, unread until opened
- [ ] 8.3 (Post-infra, manual) real SES: send to a claimed address, confirm it lands in the inbox
