## Why

Job seekers apply through many ATS platforms; the replies (confirmations,
interview invites, rejections) scatter across their personal Gmail, disconnected
from freehire. This change lets a signed-in user **connect their Gmail with one
button**, pulls in **only their ATS mail**, and shows it in an in-app **inbox
grouped by subject** — a single place to see all recruiting correspondence.
Classification/labelling is explicitly a later stage; this change stops at
connect → sync → grouped inbox.

## What Changes

- **A "Connect Gmail" OAuth flow.** A signed-in user grants `gmail.readonly` via
  Google's incremental authorization (layered on freehire's existing Google
  OAuth client, so ordinary sign-in never asks for mail access). We store a
  per-user **refresh token, encrypted at rest**, and support disconnect
  (revoke + purge). NON-GOAL now: Google app **verification / CASA audit** — the
  OAuth app runs in **testing mode** (≤100 manually-added test users, the
  "unverified app" consent screen); production verification is a later step.
- **An ATS-scoped Gmail sync worker.** A run-once cron worker reads each
  connected user's mail via the **Gmail API** (`messages.list?q=from:(<ATS
  domains>) newer_than:…` → `messages.get`), filtering to a curated ATS
  sender-domain list, and upserts full messages (headers + text/HTML bodies)
  into an `emails` store, idempotent by Gmail message id and incremental by a
  per-user sync cursor.
- **A subject-grouped inbox.** The inbox groups a user's ATS mail by
  **normalized subject** (Re:/Fwd: prefixes stripped, trimmed); each group shows
  a count + latest date, expands to its messages, and a message opens its full
  body in a sandboxed reading pane. New API + a `/my/inbox` SPA page with the
  Connect Gmail button when not yet connected.

NON-GOALS: classification/labelling of messages (a later stage); matching mail
to catalogue jobs or advancing tracker stages (that is freehire-apply's
experiment, not this); Google production verification; non-Gmail providers.

## Capabilities

### New Capabilities

- `gmail-connection`: the "Connect Gmail" incremental-OAuth flow for
  `gmail.readonly`, per-user encrypted refresh-token storage, the enabled-check
  and disconnect (revoke + purge). Layers on the existing Google OAuth client.
- `gmail-ats-sync`: the run-once cron worker that reads ATS mail via the Gmail
  API for each connected user (curated ATS sender-domain filter), upserts full
  messages into the `emails` store idempotently, and advances a per-user sync
  cursor. Gmail access behind an interface so it is testable with a fake.
- `email-inbox`: the read API for the subject-grouped inbox (groups by
  normalized subject, per-group messages, single-message body) and the
  `/my/inbox` SPA page (Connect button, grouped list, sandboxed reading pane).

### Modified Capabilities

<!-- None: this adds new capabilities. The existing Google OAuth sign-in is
     reused (incremental scope), not modified at the spec level. -->

## Impact

- **New schema (migrations/):** `gmail_connections` (per-user token + cursor)
  and `emails` (stored ATS messages). *Migration ordering caveat applies (no
  versioned runner yet — apply before deploy).*
- **New code:** an OAuth "connect" handler + callback (incremental scope);
  encrypted token storage; a Gmail API client (behind an interface) + the
  `cmd/gmail-sync` worker; a curated ATS sender-domain list; inbox handlers +
  sqlc queries; a `/my/inbox` SvelteKit page.
- **Modified:** the Google OAuth registration (add `gmail.readonly` as an
  incremental scope), `internal/config` (a token-encryption key env), the SPA
  nav.
- **New env:** a refresh-token encryption key (e.g. `GMAIL_TOKEN_KEY`); the
  Google OAuth client is reused. The Gmail OAuth app must be created/configured
  in Google Cloud (test users added).
- **Dependencies:** the Google API Go client (or a thin REST client) for the
  Gmail API and OAuth token exchange.
