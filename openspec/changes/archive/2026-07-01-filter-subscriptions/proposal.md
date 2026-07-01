## Why

Users can save searches but must re-open the site to discover new matching jobs.
We want them to subscribe to a saved search and be pushed a notification when a
matching job appears (or becomes matchable after enrichment) — starting with
Telegram. The matching must stay cheap as the number of filters and subscribers
grows, which rules out testing every new job against every filter.

## What Changes

- New per-user **subscriptions** on top of existing saved searches: subscribe a
  saved search to a delivery channel, toggle it on/off, unsubscribe.
- New **matching + delivery worker** (`cmd/notify`, run-once-and-exit cron): a
  pull/windowed model that re-runs each *distinct* filter against Meilisearch
  once per pass (cost O(distinct queries), independent of job/subscriber counts),
  records matches in a dedup ledger, and delivers one **digest per subscription
  per pass**.
- New **Telegram outbound channel**: account linking via a deep-link token + an
  inbound webhook on the Fiber server (a bot cannot message a user first), and a
  `sendMessage`-based digest sender behind a `Notifier` interface that leaves a
  seam for webhook/email channels.
- Refactor: extract the Meili filter builder out of the HTTP search handler into
  a pure function in `internal/search`, shared by the handler and the worker.

## Capabilities

### New Capabilities
- `filter-subscriptions`: subscribing a saved search to notifications, the
  windowed matching engine, the `(subscription, job)` dedup ledger, digest
  delivery with retry/dead-letter, the `Notifier` channel abstraction, and the
  `/me/subscriptions` HTTP surface.
- `telegram-notify`: outbound Telegram — linking a user's chat via a signed
  deep-link token and the inbound webhook, storing `chat_id`, and the Telegram
  digest sender. Sibling to the existing inbound `telegram-ingest`.

### Modified Capabilities
<!-- None: saved-searches and job-search requirements are unchanged; the filter-builder extraction is an implementation refactor, not a requirement change. -->

## Impact

- **New code:** `cmd/notify`, `internal/notify` (matching/delivery + `Notifier`),
  Telegram link/webhook handlers, `/me/subscriptions` handlers.
- **DB:** migration `0022` — `subscriptions`, `subscription_matches`,
  `telegram_links`; new sqlc queries.
- **Refactor:** `internal/handler/search.go` filter builder → `internal/search`.
- **Config:** `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET` (feature disabled
  when unset); reuses `JWT_SECRET`, `MEILI_URL`, `DATABASE_URL`.
- **Ops:** register the bot webhook (`setWebhook` with `secret_token`), add the
  `notify` cron (flock). No Meili reindex required (`created_at` already indexed).
- **SPA:** notify toggle + Telegram-link dialog on the saved-searches page.
- **Docker:** add the `notify` binary to the Dockerfile build + COPY list.
