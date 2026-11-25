# Notifications: channels

Part of [notifications](../notifications.md): how a message reaches a person — the `Notifier` implementations, the channel vocabulary, and what a missing or blocked channel does.

- **A new channel is a new `Notifier` implementation — one interface, three payloads.** The
  engine seam is the generic `notify.Notifier[M]` plus `notify.Router[M]` (a
  `map[channel]Notifier[M]`), instantiated as `notify.Notifier[notify.Message]` for a digest and
  `notify.Notifier[[]T]` for a reminder or nudge batch. `ErrChannelNotConfigured`,
  `ErrRecipientGone`, the destination rule (`notify.Recipient`), the claim release and the push
  wrapper (`notify.Push[M]`) are shared; the Telegram and email wrappers live in
  `internal/engage/batchnotify` because those channel packages implement `notify.Notifier` and
  cannot be imported by it. Each engine owns only its message type and its copy (a render
  function per channel). Adding a channel is therefore one wrapper plus one render function per
  engine and a wire-up in each `cmd` main that should carry it; `reminder` and `nudge` read
  destinations through the one `notify.Recipient`, so a new channel's destination rule is
  written once.
  **The fourth channel (webhook, `add-saved-search-webhooks`) answers that by landing in
  only ONE of the three engines** — a saved search's "matched a job" alert is what a
  candidate's own tooling wants pushed to it; a saved-job reminder or a lifecycle nudge is
  not. So there is no `webhooknotify` transport for `reminder`/`nudge`, and neither one's
  `cmd` main ever registers `notify.ChannelWebhook` in its `Router`. But `notify.Channels`
  is shared vocabulary (see the bullet below), and `reminder.go`'s create-time gate reads
  it too — so an account's `notification_settings.channels` will validate `"webhook"` as an
  accepted value even though nothing ever delivers over it there. This is not a new failure
  mode: it is the same "valid channel, no registered `Notifier`" soft-skip described below,
  just permanent rather than until a credential is configured. The product UI never offers
  `webhook` as a `notification_settings` choice (only the saved-search subscription control
  does), so this is reachable only by a direct API call — accepted as the cost of one shared
  vocabulary constant rather than a per-engine one.
  **Grouping made the duplication bigger, and that is the seam to watch.** `reminder` and
  `nudge` now each carry a near-identical `collect` + `deliverBatch` + `deliverChannels` trio
  — claim, validate per item, group, send once, finalize every member — differing only in
  their ledger's method names and in nudge's kind in the group key. Everything those loops
  call that is not the ledger is shared (`notify.Recipient`, `notify.Release`,
  `batchnotify.UnlinkTelegram`, `notify.JobsSnapshotOf`). The loops themselves are still two
  copies: a shared engine would have to be generic over the delivery row and the ledger's
  five statements, which is more machinery than the ~80 duplicated lines cost. A third
  batching engine is the signal to extract the loop itself.

- **Push needs no server-side credential and is therefore always registered.** Unlike Telegram
  (`TELEGRAM_BOT_TOKEN`) and email (`AWS_REGION`+`NOTIFY_EMAIL_FROM`), Expo's relay holds the
  APNs/FCM credential on its own side (set up once via `eas credentials` in `freehire-mobile`), so
  every `cmd` main registers the push notifier unconditionally — there is no "channel not
  configured" state for push, only a per-recipient one. `dest` for push is the recipient's user id
  (not a device token): a user may have zero-to-many registered devices
  (`user_push_tokens`), so `recipient()`/`notify.Recipient` soft-skips on a live `HasPushDevice`
  column (mirroring `TelegramChatID.Valid`) and `notify.Push` fans the send out to every
  device via `pushnotify.SendToDevices`, which is delivered as long as at least one device
  received it.

- **`notify.Channels` is the single source of truth** for the channel vocabulary, and
  `notify.ValidChannel` is the membership test both create-time gates use. Subscriptions and
  reminders each built their own `map[string]bool` from the slice until the test was exported.
  Add a channel there or it will be creatable but undeliverable.

- **An unconfigured channel is a soft-skip, not a failure.** `Router.Send` returns
  `ErrChannelNotConfigured` (e.g. email while SES is unset) and the engine skips it. Don't
  promote that to a delivery error — it would fail every run in environments without SES.
  **In production this makes a missing credential silent, and it has already cost a
  channel.** The mail credentials live in their own env file
  (`/opt/freehire/.env.notify`, not the `/opt/freehire/.env` every worker reads); the
  `remind` and `nudge` units did not load it, so email reminders soft-skipped from the day
  they shipped until 2026-09-01 while every run exited 0 with `failed=0`. 244 of them piled
  up across 43 people. The health signal for these workers is therefore **`soft_skips` in
  the run log, not the exit code**: a steady non-zero count against `delivered=0` is a dead
  channel, not an absence of recipients. See `freehire-ops`' `provision/host2/AGENTS.md` for
  which units must read both files.

- **A blocked Telegram bot unlinks the chat; it does not fail the delivery.** Every 403 the
  Bot API answers a send with means the chat is permanently closed (blocked, deactivated,
  bot removed), and no retry reaches it. `telegramnotify.ErrChatUnreachable` carries that up;
  `telegramnotify.SendToChat` translates it into the one `notify.ErrRecipientGone` every engine
  aliases, and the runner deletes the user's `telegram_links` row and soft-skips. **Matched on the 403, not on
  the description text** — that text is prose Telegram may reword, and a rule keyed to
  "Forbidden: bot was blocked by the user" would stop firing silently. Unlinking rather than
  disabling the subscription is the point: blocking the bot is a fact about the USER, so one
  delete makes every telegram delivery for them — digest, reminder and nudge alike — read as
  "not linked" and soft-skip, while their subscriptions survive for whenever they relink.
  Before this, one blocked subscriber failed a digest per pass forever and kept
  `freehire-notify.service` in `failed`.

- **The Telegram link token is deliberately NOT a JWT.** Telegram's deep-link `start`
  parameter allows only 1–64 chars of `[A-Za-z0-9_-]`, which a dotted ~200-char JWT
  violates, so the token is a ~43-char base64url(payload‖truncated-HMAC) blob signed with
  `JWT_SECRET` (`internal/engage/telegramnotify`). The 4096-char message cap is measured the way
  Telegram measures it — UTF-16 code units, with the widest possible "+ N more" tail
  reserved up front — because an oversized message fails deterministically, every retry
  re-fails, and the whole batch is dead-lettered. `telegramnotify.MaxMessageLen` and
  `UTF16Len` are exported together for that reason: the limit without the way to measure
  against it invites `len()`, which counts bytes. Every engine that can build a multi-job
  message needs both, which since grouping is all three.
