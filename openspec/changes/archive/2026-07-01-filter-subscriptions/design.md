## Context

`saved_searches` already stores a canonical URL query string per user, and the
HTTP search handler translates such a query into a Meilisearch filter
statelessly. Enrichment and ingest are run-once cron workers; `enrichment_outbox`
established the claim/lease/dead-letter outbox pattern with `FOR UPDATE SKIP
LOCKED`. There is no outbound Telegram code today (`internal/telegram` only
crawls). Production is a ~390k-job aggregator, so matching must not scale with job
or subscriber counts.

The full brainstorming write-up lives at
`docs/superpowers/specs/2026-06-16-filter-subscriptions-notifications-design.md`;
this document is the OpenSpec-scoped distillation.

## Goals / Non-Goals

**Goals:**
- Subscribe a saved search to notifications; deliver matching jobs to Telegram.
- Matching cost per pass = O(distinct filter queries), independent of jobs/subscribers.
- Never deliver a job to a subscription twice; survive worker crashes and overlapping cron runs.
- Leave a clean seam for webhook/email channels and for digest-frequency options.

**Non-Goals:**
- Instant (sub-cron-interval) delivery; cadence = the cron interval for now.
- Per-subscription digest frequency (instant/hourly/daily) — deferred.
- Telegram inbound commands beyond `/start` (`/stop`, `/list`) — deferred.
- A persisted filter registry or matching cursor — deferred until metrics need it.

## Decisions

- **Pull/windowed matching over push percolation.** A `cmd/notify` worker re-runs
  each distinct filter against Meili once per pass, rather than testing each new
  job against every filter. *Why:* Meili has no percolator; push is O(jobs ×
  filters) and couples the ingest hot path to notification. Pull is O(distinct
  queries) and decoupled. *Alternative considered:* event-driven fan-out on each
  `UpsertJob`/`SetJobEnrichment` — rejected for cost and coupling.

- **No `filters` table, no cursor; runtime GROUP BY + bounded re-scan.** Filter
  dedup is computed in Go (`map[canonicalQuery][]subscription`); each pass scans
  the top-N recent matches by `created_at` and relies on the ledger for "never
  twice". *Why:* a cursor does not reduce the number of Meili searches (still one
  per distinct query), only the page size — marginal benefit for real
  bookkeeping cost. Re-scanning recent jobs additionally *catches* enrich-late
  matches that a cursor would skip. *Alternative considered:* persisted `filters`
  + per-filter `cursor_at` — rejected as added complexity for no net gain.

- **`created_at` as the recency signal, not `updated_at`.** *Why:* re-crawls bump
  `updated_at`, which would resurface old jobs as "new" and spam users. `created_at`
  is stable and already a sortable Meili attribute, so **no reindex** is needed.
  Trade-off: a job enriched so late it has fallen out of the top-N window is
  missed — accepted seam.

- **`subscription_matches` is both ledger and delivery queue, with a lease.** PK
  `(subscription_id, job_id)` gives idempotent dedup; `notified_at IS NULL` rows
  are the retry queue. Delivery is a network call, so a `claimed_at` lease column
  (mirroring `enrichment_outbox`) lets the worker claim a subscription's pending
  rows in a short transaction with `FOR UPDATE SKIP LOCKED`, send the digest
  *outside* the transaction (no network call held inside a row lock), then stamp
  `notified_at`. An expired lease is reclaimable, so a crashed send is retried
  with no separate reaper. *Why the lease:* implementation revealed that holding
  `FOR UPDATE` across the Telegram `sendMessage` would be a network-call-in-
  transaction anti-pattern; the lease is the established codebase fix.

- **Telegram linking = signed deep-link token + Fiber webhook.** A bot cannot
  message first, so the user carries a `t.me/<bot>?start=<token>` link; the bot's
  `/start` arrives at a webhook on the existing server. The token is a short-TTL
  HMAC-signed credential (reusing `JWT_SECRET`, `purpose=tg-link`) so **no token
  table** is needed. The webhook is guarded by Telegram's `secret_token` header.
  *Alternative considered:* long-poll worker — rejected (breaks run-once-and-exit,
  needs a long-lived supervised process).

- **`telegram_links` as its own table.** One row per user (`chat_id`). *Why:* a
  dedicated table keeps notification concerns out of `users` and avoids touching
  the `users` `SELECT *` sqlc path (a known deploy-coupling gotcha).

- **Single `cmd/notify` binary does MATCH then DELIVER.** *Why:* unsent matches
  are the retry queue, so a Telegram outage loses nothing and no second worker is
  needed. `Notifier` interface (`Send(ctx, channel, dest, Digest) error`) selects
  the channel, mirroring `enrich.Provider`.

- **Extract the Meili filter builder into `internal/search`.** Today it is bound
  to `*fiber.Ctx` in the handler. A pure function (query/`url.Values` →
  filter) lets the handler and worker share one translation so they cannot drift.
  Targeted "improve the code you touch" refactor.

## Risks / Trade-offs

- **Burst > N new matches for one filter between passes** → oldest beyond the
  top-N window are missed. Mitigation: size N generously; tune or add a cursor if
  metrics show misses.
- **Very-late enrichment past the window** → missed match. Mitigation: enrichment
  runs continuously freshest-first, so the window rarely lapses; accepted seam.
- **Forged webhook calls** → fake links. Mitigation: mandatory `secret_token`
  verification; webhook is inert without configured credentials.
- **Telegram rate limits on broad filters** → throttling. Mitigation: digest
  collapses many matches into one message; per-subscription cap on digest size.
- **Unlinked user with active telegram subscriptions** → undeliverable. Mitigation:
  soft-skip (matches stay pending, no attempt counted) until relinked.

## Migration Plan

The bot is **@free_hire_bot** (created in @BotFather; one dedicated bot for
freehire, not shared with telagon's analytics bot). Secrets live in the deploy
env only (never in the repo).

1. Apply migration `0022_filter_subscriptions.sql` manually against prod
   (`make psql` / the documented prod migrate step — no versioned runner yet).
2. Deploy the image: the `notify` binary is already in the Dockerfile build + COPY
   list. No Meili reindex required (`created_at` is already indexed).
3. Set the deploy env (prod `.env` / freehire-ops secrets):
   - `TELEGRAM_BOT_TOKEN` — the @free_hire_bot token.
   - `TELEGRAM_BOT_USERNAME=free_hire_bot`.
   - `TELEGRAM_WEBHOOK_SECRET` — a random 32-byte hex string.
4. Register the webhook (once), pointing at the public API origin:
   ```
   curl -F "url=https://<api-origin>/api/v1/telegram/webhook" \
        -F "secret_token=$TELEGRAM_WEBHOOK_SECRET" \
        https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/setWebhook
   ```
5. Add the `notify` cron entry (e.g. every 15–30 min) under flock, as the other
   workers run: `flock -n /tmp/notify.lock docker compose run --rm app /app/notify`.

Rollback: stop the `notify` cron and `deleteWebhook`; the new tables and endpoints
are additive and inert without the worker/config.

Token hygiene: a leaked/transcript-exposed token should be rotated in @BotFather
(`/revoke`), then `TELEGRAM_BOT_TOKEN` updated and the webhook re-registered.

## Open Questions

- Final cron cadence for `notify` (15 vs 30 min) and the top-N window size — to be
  set from observed ingest/enrich rates at deploy time.
- Digest message format details (card layout, MarkdownV2 vs HTML) — settle during
  implementation of the Telegram sender.
