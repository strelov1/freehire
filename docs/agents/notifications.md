# Notifications

Three independent delivery use cases share one channel *vocabulary* (email,
Telegram, and mobile push), each with its own small `Notifier`/`Router` pair:

| Package | Use case | Worker |
|---|---|---|
| `internal/notify` | Filter subscriptions — new jobs matching a saved search | `cmd/notify` |
| `internal/reminder` | Saved-job nudges — come back before the vacancy goes stale | `cmd/remind` |
| `internal/nudge` | Lifecycle nudges — an application went silent past its stage's threshold, or moved into `interview` | `cmd/nudge` |
| `internal/emailnotify` | Email channel (SES) — implements `notify.Notifier` (the `reminder`/`nudge`-side email transports live in their own `transports.go`) | — |
| `internal/telegramnotify` | Telegram channel (Bot API, deep-link token) | — |
| `internal/pushnotify` | Mobile push channel (Expo relay) — the bare Expo transport; each of `notify`/`reminder`/`nudge` has its own thin `PushNotifier` on top, same as Telegram/email | — |

## Always true

- **A new channel is a new `Notifier` implementation — but there are THREE of them.** Each engine
  depends only on its own `Notifier` interface plus a `Router` (a `map[channel]Notifier`); the
  three interfaces share a signature and differ in payload, and `notify`/`reminder`/`nudge` each
  carry their own `Router`, `ErrChannelNotConfigured` and `recipient`. So adding a channel is a
  digest notifier, a reminder transport, a nudge transport, a case in ALL THREE `recipient`
  functions, and a wire-up in all three `cmd` mains — confirmed by push, the third channel to
  actually land this way (`add-push-notification-channel`). Collapsing the three engines into one
  is still not done: `internal/nudge` was a third **use case** over the same channels and didn't
  trigger it, and landing push — a third **channel** — didn't either, since the duplication is
  three small, near-identical `PushNotifier`s (a few lines of message-rendering each) rather than
  three copies of anything structurally significant. Revisit if a fourth channel makes the
  per-engine cost look different.
- **Push needs no server-side credential and is therefore always registered.** Unlike Telegram
  (`TELEGRAM_BOT_TOKEN`) and email (`AWS_REGION`+`NOTIFY_EMAIL_FROM`), Expo's relay holds the
  APNs/FCM credential on its own side (set up once via `eas credentials` in `freehire-mobile`), so
  every `cmd` main registers the push notifier unconditionally — there is no "channel not
  configured" state for push, only a per-recipient one. `dest` for push is the recipient's user id
  (not a device token): a user may have zero-to-many registered devices
  (`user_push_tokens`), so each engine's `recipient()` soft-skips on a live `HasPushDevice`
  column (mirroring `TelegramChatID.Valid`) and the `PushNotifier` fans the send out to every
  device via `pushnotify.SendToDevices`, which is delivered as long as at least one device
  received it.
- **`notify.Channels` is the single source of truth** for the channel vocabulary, and
  `notify.ValidChannel` is the membership test both create-time gates use. Subscriptions and
  reminders each built their own `map[string]bool` from the slice until the test was exported.
  Add a channel there or it will be creatable but undeliverable.
- **An unconfigured channel is a soft-skip, not a failure.** `Router.Send` returns
  `ErrChannelNotConfigured` (e.g. email while SES is unset) and the engine skips it. Don't
  promote that to a delivery error — it would fail every run in environments without SES.
- **Matching is O(distinct queries), not O(subscribers).** `notify.Runner.Run` groups
  subscriptions sharing a saved-search query so the search index is hit once regardless of
  how many people subscribed to it. A per-subscription loop would multiply index load by
  subscriber count.
- **The dedup ledger's primary key is what makes matching idempotent.** MATCH records
  matched jobs, DELIVER leases them and marks them notified — so re-scanning recent jobs
  never delivers twice. Preserve the two-stage split; merging match and send loses the
  guarantee.
- **`internal/nudge`'s dedup key adds an "episode key"** — the fact that must change before a
  re-notify is warranted (an application's `last_activity_at` for a follow-up nudge, a
  `stage_set` event's `occurred_at` for interview-prep) — alongside `(user, job, kind)`. This is
  what lets MATCH re-scan the same still-silent application every pass without re-pinging it: the
  episode key is unchanged, so the insert is a no-op against `application_nudges`' unique index.
  No snooze interval, no notified-count column. `internal/reminder`/`internal/notify` don't need
  this — a reminder fires once from a pre-scheduled `fire_at`, a subscription match is a distinct
  `(subscription, job)` pair already.
- `Digest.Jobs` is capped to the configured digest size while `Digest.Total` carries the true
  count, so a renderer can show the "and N more" tail. Don't derive the count from `len(Jobs)`.
- `DigestJob` deliberately carries **no internal job id** — only the public slug and URL.
- **The Telegram link token is deliberately NOT a JWT.** Telegram's deep-link `start`
  parameter allows only 1–64 chars of `[A-Za-z0-9_-]`, which a dotted ~200-char JWT
  violates, so the token is a ~43-char base64url(payload‖truncated-HMAC) blob signed with
  `JWT_SECRET` (`internal/telegramnotify`). The 4096-char message cap is measured the way
  Telegram measures it — UTF-16 code units, with the widest possible "+ N more" tail
  reserved up front — because an oversized digest fails deterministically, every retry
  re-fails, and the whole batch is dead-lettered.
- Salary fields are projected from enrichment; zero min/max or an empty currency means
  unknown, and the renderer omits the line rather than printing a zero.

## Note on naming

`internal/telegramnotify` (outbound, notifications) is the sibling of `internal/telegram`
(inbound, channel crawling for vacancies). They are unrelated concerns that both talk to the
Bot API — check which one you're in.

## Limitations

- Delivery is cron-driven and run-once; there is no retry queue. A channel outage means that
  pass's digests are skipped, and the ledger redelivers them on the next pass only if they
  were never marked notified.
