# Hosted mailbox — ops runbook

The hosted mailbox is the second inbox option (beside Connect Gmail): each user
can claim an address on our receiving domain and read mail sent there, in the same
`/my/inbox` as their Gmail mail. The application code is inert until the AWS SES
inbound pipeline below exists — **this infra is the critical path**.

Flow: `sender → MX(inbox.freehire.dev) → SES receipt rule → raw MIME in S3 +
SNS→SQS notification → cmd/mail-ingest drains SQS → parse → resolve recipient to a
mailbox → store as a hosted message`.

## 1. Migration (before deploy)

`0015_hosted_mailbox.sql` adds `mailboxes` and refactors the live `emails` table
(adds `source`/`read_at`/`s3_key`, renames `gmail_msg_id → external_id`, widens the
unique key). `emails` already holds prod Gmail data, so apply it **by hand before
deploying the new binary** — an unapplied rename makes every inbox read fail with
`42703`. Apply as the app DB role (or `ALTER … OWNER TO hire` after), per the prod
migration discipline.

## 2. Receiving domain + MX

- Use a dedicated subdomain, e.g. `inbox.freehire.dev` — **not** the apex domain,
  whose MX serves real mail. This is `MAILBOX_DOMAIN`.
- Verify the subdomain in SES **for receiving** (SES email receiving is only
  available in a subset of regions — pick one and use it for `AWS_REGION`).
- Add an `MX` record for the subdomain pointing at SES inbound:
  `10 inbound-smtp.<region>.amazonaws.com`.

## 3. SES receipt rule → S3 + SNS → SQS

- S3 bucket for raw MIME (e.g. `freehire-mail-inbound`), with a bucket policy
  allowing `ses.amazonaws.com` to `s3:PutObject`.
- SNS topic + an SQS queue subscribed to it (`MAIL_INBOUND_QUEUE_URL`); set a
  visibility timeout ≥ the worker's per-message work (a store failure leaves the
  message for redelivery). A dead-letter queue is recommended.
- SES receipt rule set (active) with one rule matching the subdomain: an **S3
  action** (bucket above, optional prefix) and an **SNS action** (the topic).
  `MAIL_INBOUND_BUCKET` is the fallback bucket when a notification omits one.

## 4. IAM for the worker

The `cmd/mail-ingest` daemon reads credentials from the default chain (instance
role / SSO), never app config. Grant it: `sqs:ReceiveMessage`,
`sqs:DeleteMessage`, `sqs:GetQueueAttributes` on the queue, and `s3:GetObject` on
the bucket.

## 5. Deploy the worker (systemd on host-2)

`cmd/mail-ingest` is a **long-lived daemon** (SES delivery is push-driven), unlike
the run-once cron workers — it long-polls SQS until `SIGTERM`. Run it as a systemd
service (not a timer), with the env below and `Restart=always`. Build it into the
release the same way as the other `cmd/*` binaries.

## 6. Environment

| var | purpose |
|-----|---------|
| `MAILBOX_DOMAIN` | receiving subdomain; also enables the claim route + status on the server |
| `AWS_REGION` | an SES-inbound-capable region |
| `MAIL_INBOUND_QUEUE_URL` | the SQS queue the SES notifications land in |
| `MAIL_INBOUND_BUCKET` | fallback S3 bucket for the raw MIME |

`MAILBOX_DOMAIN` is read by **both** the server (to offer the option and gate the
claim route) and the worker; the rest are worker-only. AWS credentials come from
the instance/SSO role. With `MAILBOX_DOMAIN` unset the option is off (claim route
unregistered, status reports unavailable); with the AWS vars unset the worker
exits cleanly (nothing to drain).

## Verify

1. `openssl s_client` / send a test mail to `<you>@inbox.freehire.dev`.
2. Confirm the object appears in S3 and a message in SQS.
3. `cmd/mail-ingest` logs the drain; the mail appears in `/my/inbox` (Mailbox
   account), unread until opened.
