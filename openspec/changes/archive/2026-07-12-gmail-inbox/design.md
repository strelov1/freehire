## Context

Job seekers' ATS replies live scattered in their personal Gmail. This change
adds, to the freehire (hire) backend, a "Connect Gmail" button, an ATS-scoped
Gmail sync, and an in-app inbox grouped by subject. It is a sibling in spirit to
the freehire-apply experiment (which used a personal app-password + IMAP and did
LLM classification), but re-homed in the open-source main project, productized
with OAuth, and deliberately simpler: **no classification, matching, or tracker
integration** — connect → sync → grouped inbox.

Constraints (hire's AGENT.md): Go + Fiber v2, PostgreSQL via sqlc (no ORM),
SvelteKit SPA under `web/`, migrations under `migrations/` (no versioned runner
yet — apply before deploy), response envelopes `{"data":…}` / `{"error":…}`,
stateless-JWT cookie auth.

## Goals / Non-Goals

**Goals:**

- One-button "Connect Gmail" via Google incremental OAuth (`gmail.readonly`).
- Per-user encrypted refresh-token storage + disconnect (revoke + purge).
- ATS-scoped sync via the Gmail API into a stored `emails` table, idempotent +
  incremental.
- `/my/inbox` grouped by normalized subject, full bodies read in a sandboxed pane.

**Non-Goals:**

- Classification/labelling of mail (a later stage).
- Matching mail to catalogue jobs or advancing tracker stages (that is
  freehire-apply's experiment).
- Google production verification / CASA audit (testing-mode test users now).
- Non-Gmail providers; sending mail.

## Decisions

### D1: Incremental OAuth on the existing Google client, not a new sign-in

"Connect Gmail" is a **separate authorization** from sign-in: it requests
`gmail.readonly` via incremental auth on the same Google OAuth client, with its
own consent, only when the user clicks Connect. *Alternative:* add the scope to
sign-in — rejected, it would force every sign-in to request mail access.

### D2: Gmail REST API, not IMAP

With OAuth, the Gmail API is the natural transport: server-side `messages.list`
with a `q=from:(<ATS domains>) newer_than:…` query does the ATS filtering and
incrementality in one call, then `messages.get` fetches full content.
*Alternative:* IMAP over SASL XOAUTH2 — works, but re-implements filtering the
Gmail query gives for free and is a poorer fit for OAuth tokens.

### D3: Refresh token encrypted at rest, disconnect purges

Store only an encrypted refresh token (server key from env), never returned to
the client; access tokens are minted per run and never stored. Disconnect
revokes the grant with Google and purges the token **and** the user's synced
mail. *Alternative:* store plaintext — rejected (a DB leak would expose live
mailbox access).

### D4: Store full bodies, dedup by Gmail message id

The `emails` table holds headers + text/HTML bodies so the inbox reads in-app;
`gmail_msg_id` is unique for idempotent re-sync. A per-user cursor (Gmail
`historyId` or a received-time watermark) drives incrementality. *Alternative:*
metadata + deep-link to Gmail — rejected by the product decision (read in-app).

### D5: Group by normalized subject

The inbox groups by a normalized subject: strip leading `Re:`/`Fwd:` (and
locale variants) and trim/collapse whitespace, case-insensitively. Grouping is a
pure function, unit-tested; the API groups on a stored `subject_norm` column so
the grouping key is indexable. *Alternative:* thread by `In-Reply-To` — more
accurate for real threads but ATS mail is often not threaded, and the product
asked for subject grouping.

### D6: Gmail access behind an interface

The Gmail client sits behind a small interface (list ATS message ids since
cursor; get a message) so the sync worker is unit-tested with a fake and no live
Google. Token exchange/refresh is likewise adapter-isolated.

## Risks / Trade-offs

- **Restricted-scope verification** → Deferred: testing-mode test users
  (≤100) now; production needs Google verification + CASA audit. Surfaced to
  the user; a clear "not yet available" message for non-test users.
- **Storing users' mail bodies** → Privacy/volume/GDPR load. Mitigated by
  disconnect-purges-all and (seam) a retention policy; ATS-only scoping bounds
  volume.
- **Refresh-token leak** → Encrypted at rest; revoke-on-disconnect; never
  returned to the client.
- **Gmail API quotas** → Batch requests + backoff; best-effort per user so one
  user's quota error never aborts the run.
- **Cursor staleness / Gmail historyId expiry** → Fall back to a received-time
  watermark backfill; dedup by message id absorbs overlap.

## Migration Plan

- New migrations: `gmail_connections` (user_id, email, refresh_token_enc,
  connected_at, last_synced_at, sync_cursor, status) and `emails` (user_id,
  gmail_msg_id UNIQUE, thread_id, from, subject, subject_norm, body_text,
  body_html, received_at). Apply before deploy (no versioned runner).
- New env: a refresh-token encryption key (`GMAIL_TOKEN_KEY`). The Google OAuth
  app must be configured in Google Cloud (Gmail scope + test users) before the
  connect flow works; without it the button surfaces "not available".
- Rollback: the feature is additive; disconnect purges per-user data.

## Open Questions

- Cursor mechanism: Gmail `historyId` (efficient, can expire) vs an
  `internalDate` watermark (simple, robust) — start with the watermark, revisit.
- Sync cadence (cron interval) — tune after observing volume/quota.
- Retention policy for stored bodies (beyond disconnect-purge) — a later seam.
