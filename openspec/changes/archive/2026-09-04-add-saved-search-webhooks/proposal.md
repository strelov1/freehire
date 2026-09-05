## Why

Saved-search alerts currently only reach a candidate through email or Telegram. Some
candidates want matched jobs pushed into their own tooling (a personal script, a
Zapier/Make flow, a Slack app they run themselves) instead of reading a message —
that requires an outbound HTTP callback the platform doesn't offer today.

## What Changes

- Add `webhook` as a fourth saved-search notification channel, alongside the
  existing `telegram` and `email` channels — a saved search subscribes to it the
  same way it subscribes to the others.
- Add one webhook destination per account (URL + HMAC secret), managed under the
  signed-in user's account settings: create/rotate (returns the plaintext secret
  once), view (never returns the secret again), enable/disable, delete.
- Deliver by reusing the existing subscription digest pipeline unchanged (match
  detection, dedup ledger, claim-under-concurrency, retry with attempt count,
  dead-letter) — the new work is a `Notifier` implementation that HMAC-signs the
  digest and POSTs it through the SSRF-safe HTTP client, since the destination is
  a URL the user supplies.
- A definitive `410 Gone` response from the destination disables the webhook
  immediately (mirrors the existing Telegram unlink-on-403 behavior); any other
  failure follows the existing per-match retry/dead-letter path unchanged.
- Add an account-settings UI section to configure the webhook (URL, enable
  toggle, secret rotation with a one-time reveal) and extend the existing
  saved-search subscription controls to offer the `webhook` channel.

## Capabilities

### New Capabilities
- `webhook-notifications`: account-level webhook destination (URL + HMAC secret,
  encrypted at rest, create/rotate/disable), the signing `Notifier` that delivers
  a subscription digest as a signed HTTP POST, and the settings UI to manage it.

### Modified Capabilities
- `filter-subscriptions`: the set of supported subscription channels grows from
  (`telegram`, `email`) to (`telegram`, `email`, `webhook`); recipient resolution
  gains a `webhook` case that reads the account's webhook destination instead of
  a per-subscription stored address.

## Impact

- New migration: `webhook_configs` table (one row per account: URL, encrypted
  secret, enabled flag, disabled timestamp).
- New env var for the secret's encryption key (reusing
  `internal/platform/tokencrypt`, the same AES-256-GCM helper the Gmail/Calendar
  OAuth token store uses, with its own dedicated key rather than sharing
  `GMAIL_TOKEN_KEY`).
- New package `internal/engage/webhooknotify` (the `Notifier`), modeled on
  `internal/engage/telegramnotify`/`emailnotify`.
- Changes to `internal/engage/notify` (new channel constant, `Store` interface
  method(s) for the webhook destination, recipient resolution, the
  `ErrRecipientGone`-on-410 mapping) and `cmd/notify/main.go` (wiring the new
  notifier into the router).
- New `internal/api/handler` endpoints under `/api/v1/me` for webhook config
  CRUD, using `internal/platform/safehttp` for the outbound delivery call.
- New account-settings page/section in `web/` plus a channel option added to the
  existing saved-search subscription controls.
