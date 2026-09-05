## 1. Data model & encryption

- [x] 1.1 Add migration `013X_webhook_configs.sql` creating `webhook_configs` per design.md (PK `user_id`, `url`, `secret_encrypted`, `enabled`, `created_at`, `updated_at`, `last_success_at`, `disabled_at`)
- [x] 1.2 Add `WEBHOOK_SECRET_KEY` (base64, 32 bytes) to `internal/platform/config` alongside `GmailTokenKey`, failing fast on an invalid/missing value the same way; add it to `.env.example`
- [x] 1.3 Add `internal/platform/db/queries/webhooks.sql`: `UpsertWebhookConfig` (insert-or-rotate on `user_id` conflict), `GetWebhookConfig`, `EnableWebhookConfig`/`DisableWebhookConfig` (split instead of one boolean setter, matching the codebase's specific-purpose-mutation convention), `RecordWebhookDeliverySuccess`, `DeleteWebhookConfig`; run `make sqlc`

## 2. Delivery engine wiring (`internal/engage/notify`)

- [x] 2.1 Add `const ChannelWebhook = "webhook"` and append it to `Channels`
- [x] 2.2 Extend the `GetSubscriptionForDelivery` query with a `LEFT JOIN webhook_configs` and the three new columns (`webhook_url`, `webhook_secret_encrypted`, `webhook_enabled`); run `make sqlc`
- [x] 2.3 Add a `case ChannelWebhook` to `recipient()`: soft-skip unless enabled+URL present, otherwise return the JSON-encoded `{url, secret_encrypted}` dest
- [x] 2.4 Add `DisableWebhookConfig(ctx, userID) (int64, error)` to the `Store` interface
- [x] 2.5 Fix `deliverOne`'s `ErrRecipientGone` branch (currently hardcoded to `unlinkTelegram`) to dispatch on `info.Channel`, adding a `disableWebhook` helper mirroring `unlinkTelegram` for the webhook case

## 3. Webhook notifier (`internal/engage/webhooknotify`, new package)

- [x] 3.1 Scaffold the package: `Notifier` struct holding a `*tokencrypt.Cipher` and an `*http.Client` built on `internal/platform/safehttp`
- [x] 3.2 Implement `Send`: unmarshal `dest`, decrypt the secret, JSON-marshal the digest body (reusing `DigestJob`), compute `hex(hmac.SHA256(secret, body))`, POST with `X-Freehire-Signature: sha256=<hex>` and a fixed timeout
- [x] 3.3 Map a `410` response to a wrapped `notify.ErrRecipientGone`; re-validate the URL scheme (`http`/`https`) before sending as defense in depth
- [x] 3.4 Register `webhooknotify.NewNotifier(...)` in the router built by `cmd/notify/main.go`
- [x] 3.5 Add `webhooknotify` to the package list in `internal/engage/AGENTS.md`

## 4. API endpoints (`internal/api/handler`)

- [x] 4.1 New `webhook.go`: `POST /api/v1/me/webhook` — validate URL scheme, generate an opaque secret, encrypt it, upsert, return the plaintext secret once
- [x] 4.2 `GET /api/v1/me/webhook` — metadata only (`url`, `enabled`, `created_at`, `last_success_at`, `disabled_at`), `{"data": null}` when unconfigured
- [x] 4.3 `PATCH /api/v1/me/webhook` — toggle `enabled` without rotating the secret
- [x] 4.4 `DELETE /api/v1/me/webhook` — remove the row
- [x] 4.5 Register all four routes under `RequireAuth` (cookie only, no API key)

## 5. Frontend (`web/`)

- [x] 5.1 Add `WebhookConfig`/`CreatedWebhookConfig` types and the four API calls to `web/src/lib/api.ts`
- [x] 5.2 New `web/src/lib/components/WebhookSettingsView.svelte` using the reveal-once pattern from `ApiKeysView.svelte`
- [x] 5.3 New route `web/src/routes/my/webhook/+page.svelte` and a nav entry in `AccountNavRail.svelte`
- [x] 5.4 Add a `webhook` chip to `web/src/lib/components/filters/AlertChannels.svelte`

## 6. Tests

- [x] 6.1 Unit tests for `recipient()`'s webhook case (enabled+configured, disabled, unconfigured) — done alongside 2.3
- [x] 6.2 Unit tests for `webhooknotify.Send` (signature correctness against a fixed secret/body, `410` → `ErrRecipientGone`, rejection of a non-http(s) or private-address URL) — done alongside 3.1-3.3
- [x] 6.3 Unit test for `deliverOne`'s channel-dispatched `ErrRecipientGone` handling (a webhook 410 disables the webhook config and leaves any Telegram link untouched, and vice versa) — done alongside 2.5
- [x] 6.4 Integration tests (`-tags=integration`) for the four `/api/v1/me/webhook` handlers: create/rotate returns the secret once, get/list never does, patch toggles, delete removes, all require the session cookie
- [x] 6.5 Manually verify the frontend flow end-to-end (create webhook, copy secret, subscribe a saved search to the `webhook` channel, toggle disable/enable) via the `run` skill before calling the UI work done — verified live via Playwright against `cmd/server` + `pnpm dev` + Postgres/Meilisearch in Docker: create shows the reveal-once secret panel, rotate issues a new one, disable/enable toggles correctly (`Disabled · since now`), and the saved search's alert chips show `✓ Webhook` when subscribed and toggle off/on cleanly — zero browser console errors throughout

## 7. Docs

- [x] 7.1 Confirm no other `AGENTS.md` enumerates the notification channel list and needs the `webhook` addition (check `docs/agents/notifications.md`) — found and documented a real consequence: `notify.Channels` is shared with `reminder`'s create-time gate, so `webhook` validates there too despite no transport ever being registered for it (permanent soft-skip, same failure mode as an unconfigured channel)
