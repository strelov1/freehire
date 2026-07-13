## Context

freehire (hire) already has a Gmail-connect path: `internal/gmailsync` (OAuth
`gmail.readonly`, ATS-scoped sync) writes stored mail into a **Gmail-specific**
`emails` table (`gmail_msg_id NOT NULL UNIQUE (user_id, gmail_msg_id)`), and
`/my/inbox` reads it grouped by normalized subject. That path is blocked for the
general public by Google's restricted-scope verification (test-users only).

This change adds a **second, Google-independent option**: a per-user mailbox on our
own receiving domain, fed by AWS SES inbound — the same architecture the
freehire-apply experiment used (`internal/mailbox` for address allocation,
`internal/ingest` for the SES→S3→SQS drain). The two options must sit **side by
side** in one inbox, so the existing Gmail store is refactored into a
source-agnostic message store rather than growing a parallel table.

Constraints (hire's AGENT.md): Go + Fiber v2, PostgreSQL via sqlc (no ORM),
SvelteKit SPA under `web/`, migrations under `migrations/` (no versioned runner —
apply before deploy), response envelopes `{"data":…}` / `{"error":…}`, stateless-JWT
cookie auth. MVP-fluid: reshaping the Gmail store to fit both sources cleanly is
preferred over a special-case second table.

## Goals / Non-Goals

**Goals:**

- A per-user address `<handle>@<MAIL_DOMAIN>` (receiving subdomain), collision-suffixed.
- SES-backed inbound ingest: receive → parse MIME → resolve recipient → store,
  idempotent by RFC `Message-ID`, best-effort per message, testable over a fake source.
- One **unified mail store** both the Gmail worker and the SES worker write through;
  the inbox lists both sources together.
- Read/unread state; opening a message marks it read.
- `/my/inbox` offers both options ("Connect Gmail" + "Get a freehire mailbox") and
  shows the claimed address.

**Non-Goals:**

- Classification/labelling, job matching, tracker-stage advancement.
- **Sending** applications from the hosted address (the full apply loop).
- Multiple mailboxes per user; non-SES inbound; a versioned migration runner.
- Google production verification (unrelated to this option).

## Decisions

### D1: Unify the mail store, refactor Gmail into it (not a parallel table)

The Gmail-specific `emails` table is generalized in place: add `source TEXT NOT NULL
DEFAULT 'gmail'` (`gmail` | `hosted`), **rename `gmail_msg_id` → `external_id`**
(Gmail message id for `gmail`, RFC `Message-ID` for `hosted`), add nullable
`s3_key` (hosted raw-MIME pointer) and `read_at TIMESTAMPTZ`. Uniqueness widens from
`(user_id, gmail_msg_id)` to `(user_id, source, external_id)`. `0014` is already live
on prod, so this ships as an in-place `ALTER` preserving existing Gmail rows (the
default `source='gmail'` backfills them). *Alternative:* a separate `messages` table
like apply — rejected, it would force every inbox read/handler to union two shapes and
duplicate the reading pane.

### D2: SES inbound, mirroring freehire-apply's proven ingest

Inbound uses AWS SES receipt rules → raw MIME in S3 → SNS→SQS. The worker long-polls
SQS, fetches the S3 object, parses the MIME, resolves the `To:`/recipient to a
`mailboxes.address` → owning user, and upserts a `hosted` message. Ported from apply's
`internal/ingest` (SESSource + parse + resolve). *Alternative:* an inbound-email SaaS
(Postmark/Mailgun) — rejected, we already own an SES identity and the apply code proves
the SES path; no new vendor.

### D3: Receiving subdomain with its own MX, not the apex domain

`freehire.dev`'s MX serves real mail and cannot be repurposed. The mailbox domain is a
dedicated **receiving subdomain** `inbox.freehire.dev` (default `MAIL_DOMAIN`) whose MX
points at SES inbound — exactly why apply used a separate domain (`careermails.net`).
Address = `<handle>@inbox.freehire.dev`, handle derived from the user's email local-part,
lowercased, `[a-z0-9.-]` only, collision-suffixed (`-2`, `-3`, …). *Alternative:* a brand
new domain — viable but an extra registration; a subdomain keeps it "on our domain" as asked.

### D4: Ingest behind an InboundSource interface; daemon, not cron

`cmd/mail-ingest` is a long-lived poller (SQS long-poll loop), unlike the run-once cron
workers — SES delivery is push-driven, so a daemon draining the queue fits. The AWS
transport sits behind an `InboundSource` interface (`Receive`/`Ack`) so the
parse/resolve/store logic is unit-tested over a fake with no live AWS. Deployed as a
systemd service on host-2.

### D5: Address allocation is explicit and idempotent

A user claims a mailbox via `POST /me/mailbox` (allocate-or-return; re-claim returns the
existing address). Allocation retries the collision suffix inside a transaction on the
`address` unique violation. `GET /me/mailbox` returns the address (or null); the SPA shows
it. Release (`DELETE`) drops the mailbox and its hosted messages (Gmail mail untouched).

### D6: Inbox is source-agnostic with an account switcher

The listing/group/get queries select across `source` and accept an **optional `source`
filter**, so the inbox can show one account's mail or all of it in a single query. The SPA
renders an **account switcher** — `All` · `Gmail` · `freehire mailbox` — that sets that
filter; a source the user hasn't connected is simply hidden from the switcher, and with one
source connected the switcher can collapse to just that account. A message's wire shape gains
`source` and `read`. The Gmail deep-link stays Gmail-only (hosted messages have no Gmail URL).
`GetEmail` stamps `read_at` on open. The unified store makes both the merged "All" view and a
per-account view the same query with an optional predicate — no separate code path per source.
This also generalizes to multiple accounts later (the switcher becomes a list of connected
accounts) without reshaping the store.

## Risks / Trade-offs

- **SES inbound infra is the critical path.** MX + receipt rules + S3 + SQS + IAM +
  region support + domain-verified-for-receiving are ops work outside the repo; the code
  is inert until they exist. Mitigated by fake-source tests (ship + verify logic early) and
  a clear infra checklist; live enablement gated on freehire-ops.
- **Refactoring a live table.** `0014` is on prod with the user's synced mail. The
  generalizing `ALTER` must preserve rows and the app must be deployed in lockstep with the
  migration (unapplied-migration → `42703` on every inbox read, per the deploy gotcha).
  Mitigated: additive columns + a rename with `source` default; apply migration before deploy.
- **Spam/abuse to a public receiving address.** An open mailbox invites spam. Mitigated by
  ATS-domain scoping at display time (reuse `gmailsync.IsATSSender`) as a later seam;
  for now store all received mail (the address is user-chosen and low-volume).
- **Storing users' mail bodies + raw MIME in S3.** Privacy/volume. Mitigated by
  release-purges-hosted-mail and (seam) a retention policy.
- **Idempotency on missing Message-ID.** Some senders omit `Message-ID`; the worker
  synthesizes a stable key from the S3 object key (apply's approach) so re-delivery dedups.

## Migration Plan

- `mailboxes (id, user_id UNIQUE, address UNIQUE, created_at)`.
- Refactor `emails`: `ADD COLUMN source … DEFAULT 'gmail'`, `RENAME gmail_msg_id →
  external_id`, `ADD COLUMN s3_key TEXT`, `ADD COLUMN read_at TIMESTAMPTZ`; drop old unique,
  add `UNIQUE (user_id, source, external_id)`. Apply to prod by hand before deploy.
- New env `MAIL_DOMAIN` / `AWS_REGION` / `MAIL_INBOUND_QUEUE_URL` / `MAIL_INBOUND_BUCKET`;
  feature-gated (absent → mailbox routes report unavailable, like the Gmail `available` flag).
- Rollback: additive + a column rename; hosted rows are removable, Gmail rows keep working.

## Open Questions

- Final mailbox domain: `inbox.freehire.dev` (default) vs a dedicated domain — DNS/SES choice.
- SES inbound region (must be a receiving-capable region; may differ from the app region).
- Whether to ATS-scope hosted mail at ingest or store-all + filter-at-display (start store-all).
