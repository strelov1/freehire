## Context

`internal/engage/notify` (`Runner.Run`) already does MATCH-then-DELIVER for the
`telegram`/`email`/`push` channels: it groups active subscriptions by distinct
query, records matches in `subscription_matches` (a dedup ledger with
`attempts`/`failed_at` columns), then claims pending matches under skip-locked
concurrency and sends one digest per subscription through the `Notifier`
interface, dispatched by a `Router map[string]Notifier`
(`internal/engage/notify/router.go`). Recipient resolution
(`internal/engage/notify/notify.go:242`, `recipient()`) reads a per-channel
destination from `GetSubscriptionForDeliveryRow` — Telegram's `chat_id`, the
account's live email — never a value stored on the subscription itself.
`RecordMatchDeliveryFailure` increments `attempts` and sets `failed_at` once
`Config.MaxAttempts` (5) is reached; a distinct sentinel, `ErrRecipientGone`,
short-circuits that counting for a destination that is *definitively* gone
(Telegram 403 → `deliverOne` unlinks the chat and soft-skips instead of
dead-lettering). This existing machinery is confirmed sufficient for webhook
delivery too (see proposal.md) — no new outbox table or worker.

Secrets the system must read back later (as opposed to API keys, which are only
ever compared by hash) already have a home: `internal/platform/tokencrypt.Cipher`
(AES-256-GCM), used today to store Gmail/Calendar OAuth refresh tokens, keyed by
`GMAIL_TOKEN_KEY`.

## Goals / Non-Goals

**Goals:**
- Add `webhook` as a fourth channel inside the existing subscription
  match/delivery engine, reusing its ledger, retry, and dead-letter behavior
  unchanged.
- Store one webhook destination per account (URL + a secret the system can
  recover for signing) and manage it through `/api/v1/me/webhook` + an account
  settings screen.

**Non-Goals:**
- No replay protection (timestamp/nonce in the signature) — the payload is a
  read-only digest, not a state-changing action, and this can be added later
  without a spec or schema change if a receiver needs it.
- No per-saved-search webhook URL — the destination is account-wide; only the
  channel subscription (on/off, per saved search) is per-search, matching the
  chosen product shape.
- No webhook delivery history/logs UI beyond `last_success_at`/`disabled_at` —
  the existing `subscription_matches.last_error` already carries the last
  failure per match for support/debugging.

## Decisions

### Data model: `webhook_configs`, one row per account

New migration (next number after `0131`) adds:

```sql
CREATE TABLE webhook_configs (
    user_id           bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    url               text NOT NULL,
    secret_encrypted  text NOT NULL,
    enabled           boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    last_success_at   timestamptz,
    disabled_at       timestamptz
);
```

`user_id` as the primary key enforces "one per account" at the schema level, so
create-or-rotate is a single `INSERT ... ON CONFLICT (user_id) DO UPDATE`
(`internal/platform/db/queries/webhooks.sql`, new file). `secret_encrypted` is
`tokencrypt.Cipher.Encrypt` output (base64 `nonce||ciphertext`), decrypted only
at delivery time. No `consecutive_failures` counter: the only automatic
disablement is the definitive 410 signal (see below), so there is nothing to
count — this mirrors Telegram, which unlinks on the first `403` rather than
after N failures.

**Alternative considered:** store the secret hashed like `api_keys`. Rejected —
API keys are only ever *compared* (the caller re-presents the plaintext); a
webhook secret must be *reproduced* by the server to sign every outbound
request, which a one-way hash cannot do.

### Encryption key: reuse `tokencrypt`, a dedicated env var

Add `WEBHOOK_SECRET_KEY` (base64, 32 bytes) alongside `GMAIL_TOKEN_KEY` in
`internal/platform/config`, decoded through the same `decodeKey` helper: an
unset or malformed value degrades to `nil` rather than failing startup,
exactly like `GmailTokenKey`. Both the `/api/v1/me/webhook` routes and the
`webhook` entry in the delivery router are registered only when the key
decodes to 32 bytes — the same `len(cfg.GmailTokenKey) != 32` guard
`cmd/server/main.go`'s Gmail wiring already uses — since, unlike email/Telegram
(which only need their credential at delivery time), creating a webhook
destination needs the cipher immediately, to encrypt the secret it stores. A
dedicated key keeps the webhook-secret domain
and the Gmail-token domain independently rotatable — sharing `GMAIL_TOKEN_KEY`
would mean a Gmail-driven key rotation silently re-scopes webhook secrets too,
and vice versa.

### Channel wiring in `internal/engage/notify`

- Add `const ChannelWebhook = "webhook"` and append it to `Channels` — the only
  change `internal/engage/subscription` needs, since `ValidChannel` already
  reads that slice.
- `GetSubscriptionForDelivery` (`internal/platform/db/queries/subscriptions.sql`)
  gains a `LEFT JOIN webhook_configs ON webhook_configs.user_id = subscriptions.user_id`
  and three columns: `webhook_url`, `webhook_secret_encrypted`, `webhook_enabled`.
- `recipient()` gains a `case ChannelWebhook`: not deliverable (soft-skip) unless
  `webhook_enabled` is true and `webhook_url` is set; otherwise it returns a
  compact JSON string `{"url":"...","secret_encrypted":"..."}` as `dest`. The
  encrypted form travels through `dest` unchanged — decryption happens inside
  the notifier, not in `recipient()`, because `recipient()` is a pure function
  with no cipher dependency and the notifier is where the plaintext secret is
  actually needed (to sign).
- `Store` gains `DisableWebhookConfig(ctx, userID) (int64, error)`, the webhook
  analog of `DeleteTelegramLink`.
- **`deliverOne`'s `ErrRecipientGone` branch currently calls `unlinkTelegram`
  unconditionally** (`internal/engage/notify/deliver.go`) — it was never
  channel-switched because only Telegram produced this error before. It must
  become a small dispatch on `info.Channel` (Telegram → `unlinkTelegram`,
  webhook → a new `disableWebhook`) so a webhook's 410 cannot mistakenly unlink
  the user's Telegram link. This is a required fix, not an incidental cleanup —
  without it the two channels' "recipient gone" handling collide.

### `internal/engage/webhooknotify` (new package)

Modeled on `internal/engage/telegramnotify`/`emailnotify`: one `Notifier`
implementation.

- `Send(ctx, channel, dest string, d notify.Digest) error` unmarshals `dest`,
  decrypts the secret with the injected `*tokencrypt.Cipher`, JSON-marshals a
  body (`{saved_search_name, total, jobs: [...same DigestJob shape already used
  for email/telegram rendering]}`), computes `hex(hmac.SHA256(secret, body))`,
  and POSTs it with header `X-Freehire-Signature: sha256=<hex>` plus
  `Content-Type: application/json`.
- The HTTP client is built from `internal/platform/safehttp` (SSRF-guarded
  transport) with a fixed timeout (10s, matching the other outbound
  notifiers' bound).
- Response mapping: `2xx` → success; `410` → wrapped `notify.ErrRecipientGone`
  (per the package-per-channel-sentinel convention `router.go` documents);
  anything else (network error, timeout, other non-2xx) → a plain error, which
  `deliverOne` already turns into the standard attempt-count/dead-letter path.
- URL scheme is re-validated defensively before every send (`http`/`https`
  only) even though the API layer already enforces it at creation — the two
  checks guard different things (creation-time input validation vs.
  delivery-time defense in depth) and the second is cheap.

### API endpoints (`internal/api/handler`, new file `webhook.go`)

All under `RequireAuth` (cookie only, matching subscription and API-key
management — never an API key):

| Method | Path | Behavior |
|---|---|---|
| `POST` | `/api/v1/me/webhook` | Create-or-rotate. Body `{url}`. Validates scheme. Generates a new opaque secret, encrypts it, upserts. Response includes the plaintext secret once. |
| `GET` | `/api/v1/me/webhook` | `{"data": {url, enabled, created_at, last_success_at, disabled_at} \| null}` — never the secret. |
| `PATCH` | `/api/v1/me/webhook` | Body `{enabled}`. Toggles without rotating. |
| `DELETE` | `/api/v1/me/webhook` | Removes the row entirely. |

### Frontend

- New `web/src/lib/components/WebhookSettingsView.svelte`, following
  `ApiKeysView.svelte`'s reveal-once pattern: the plaintext secret from a
  create/rotate response is held in local state and shown once with a copy
  button, never re-fetched.
- New route `web/src/routes/my/webhook/+page.svelte` plus a nav entry in
  `AccountNavRail.svelte` (its own section, distinct from "Notifications",
  which — per `notification-settings`'s spec purpose — governs reminders and
  nudges, not saved-search subscriptions).
- `web/src/lib/components/filters/AlertChannels.svelte` gains a `webhook` chip
  beside `telegram`/`email`/`push`, calling the existing
  `notifications.subscribe/unsubscribe(savedSearchId, "webhook")` — no new
  frontend subscription logic, since the channel enum is the only thing that
  changed on that path.
- `web/src/lib/api.ts` gains the four webhook calls and `WebhookConfig`/
  `CreatedWebhookConfig` types, mirroring the `ApiKey`/`CreatedApiKey` pair.

## Risks / Trade-offs

- **A receiver treats `410` casually.** [Risk] A misbehaving endpoint that
  returns `410` for a transient reason gets disabled immediately, worse than
  the bounded-retry path other errors get. → [Mitigation] `410 Gone` is a
  deliberate, rarely-emitted status specifically for "this resource is
  intentionally, permanently retired" (the same convention GitHub/Stripe/Slack
  webhook receivers use to unsubscribe themselves); re-enabling is a single
  toggle in settings if it ever fires wrongly.
- **No replay protection.** [Risk] A captured request/signature pair can be
  replayed. → [Mitigation] accepted as a Non-Goal — the payload is a read-only
  job digest, not an action, so a replay discloses nothing the original
  delivery didn't already, and the URL itself is only known to the account
  owner.
- **One secret, no per-subscription scoping.** [Risk] Any saved search the
  account subscribes to `webhook` shares the one destination/secret — a
  receiver cannot distinguish which saved search triggered a delivery except
  by reading `saved_search_name` in the body. → [Mitigation] acceptable per the
  chosen product shape (one webhook per account); a per-search secret would
  need a different data model and was explicitly ruled out during scoping.

## Migration Plan

1. Ship the migration + sqlc regen, `WEBHOOK_SECRET_KEY` added to deploy env
   (server + `cmd/notify`) — additive, no backfill.
2. Ship the backend (channel constant, notifier, API endpoints) behind normal
   deploy — `webhook` becomes a valid subscription channel the moment
   `notify.Channels` includes it, but nothing uses it until the frontend ships.
3. Ship the frontend (settings screen + `AlertChannels` chip).
4. Rollback is a plain revert at any step — no data migration to undo (the new
   table and column stay empty/harmless if unused) and dropping `webhook` from
   `Channels` at the app layer would only stop *new* subscriptions, which is
   the intentional pre-existing shape of that allowlist (not a DB constraint).
