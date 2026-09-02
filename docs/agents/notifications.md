# Notifications

Three independent delivery use cases share one channel *vocabulary* (email,
Telegram, and mobile push), each with its own small `Notifier`/`Router` pair:

| Package | Use case | Worker |
|---|---|---|
| `internal/engage/notify` | Filter subscriptions — new jobs matching a saved search | `cmd/notify` |
| `internal/engage/reminder` | Saved-job nudges — come back before the vacancy goes stale | `cmd/remind` |
| `internal/engage/nudge` | Lifecycle nudges — an application went silent past its stage's threshold, or moved into `interview` | `cmd/nudge` |
| `internal/engage/emailnotify` | Email channel (SES) — implements `notify.Notifier` (the `reminder`/`nudge`-side email transports live in their own `transports.go`) | — |
| `internal/engage/telegramnotify` | Telegram channel (Bot API, deep-link token) | — |
| `internal/engage/pushnotify` | Mobile push channel (Expo relay) — the bare Expo transport; each of `notify`/`reminder`/`nudge` has its own thin `PushNotifier` on top, same as Telegram/email | — |

## Always true

- **All three engines deliver in GROUPS, and a group is the unit of everything.**
  `notify` groups a subscription's matched jobs; `internal/engage/reminder` groups an
  account's due reminders; `internal/engage/nudge` groups an account's due nudges **of one
  kind**, and `internal/engage/reminder` additionally splits on the CHANNEL SET (see the
  bullet below). Every `Notifier.Send` therefore takes a whole group — a `notify.Digest`
  for subscriptions, a slice for the other two — and none of them takes a single item, so
  no per-item send path is left for a channel to keep using. That is the point: eight
  saves in a day were eight emails three days later. The kinds stay apart because "your
  application went quiet" and "prepare for your interview" are different errands with
  different call-to-actions; merging them would need a mail that says neither. One send
  outcome decides the whole group: a failure records an attempt against every member and
  the group returns whole on a later pass, because a partial result would need a second
  delivery ledger to describe and nothing reads one. A group of ONE must stay
  byte-identical to the pre-grouping message — that is what makes the change invisible to
  everyone it doesn't help.
- **A reminder's channels are a SNAPSHOT, so they are part of its batch key.**
  `job_reminders.channels` is frozen when the reminder is scheduled — migration 0034 says
  why: "a later rule edit never rewrites a pending reminder". So an account that changed
  its rule between two saves has two genuinely different deliveries due, and grouping on
  the account alone would send one of them over the other's channels and stamp it
  delivered anyway. The key is `(user_id, sorted channel set)`; only the KEY is sorted, so
  the send still walks the first member's own slice. `internal/engage/nudge` has no such
  split: `GetNudgeForDelivery` reads `notification_settings.channels` live, which IS an
  account property.
- **A reminder's `fire_at` is rounded forward to the account's notification hour**
  (`notification_settings.digest_time` in the account's timezone; 09:00 and UTC when
  unset). Grouping alone would have bought the reminder engine almost nothing: `fire_at`
  was save + 3 days exactly and `freehire-remind` ticks every 15 minutes, so two saves
  hours apart landed in different passes. **It collapses a day onto TWO fire times, not
  one** — the delay floor and a fixed hour disagree for saves that straddle that hour, so
  a day's saves split at it. Two messages instead of eight is the win; do not write down
  a promise of one. `internal/engage/nudge` needs no such rounding —
  it has no `fire_at`, and MATCH+DELIVER share a pass, so everything an account has
  pending is already together.
- **A new channel is a new `Notifier` implementation — but there are THREE of them.** Each engine
  depends only on its own `Notifier` interface plus a `Router` (a `map[channel]Notifier`); the
  three interfaces share a signature and differ in payload, and `notify`/`reminder`/`nudge` each
  carry their own `Router`, `ErrChannelNotConfigured` and `recipient`. So adding a channel is a
  digest notifier, a reminder transport, a nudge transport, a case in ALL THREE `recipient`
  functions, and a wire-up in all three `cmd` mains — confirmed by push, the third channel to
  actually land this way (`add-push-notification-channel`). Collapsing the three engines into one
  is still not done: `internal/engage/nudge` was a third **use case** over the same channels and didn't
  trigger it, and landing push — a third **channel** — didn't either, since the duplication is
  three small, near-identical `PushNotifier`s (a few lines of message-rendering each) rather than
  three copies of anything structurally significant. Revisit if a fourth channel makes the
  per-engine cost look different.
  **Grouping made the duplication bigger, and that is the seam to watch.** `reminder` and
  `nudge` now each carry a near-identical `collect` + `deliverBatch` pair — claim, validate
  per item, group, send once, finalize every member — differing only in their ledger's
  method names and in nudge's kind in the group key. Two copies is not yet an abstraction:
  a shared engine would have to be generic over the delivery row, the ledger's five
  statements and the message type, which is more machinery than the ~80 duplicated lines
  cost. What IS shared is what would silently diverge — `notify.ListLimit` and `Listed`,
  `notify.SnapshotJob`/`JobsSnapshot`, and `telegramnotify.MaxMessageLen`/`UTF16Len`. A
  third batching engine is the signal to extract the loop itself.
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
  **In production this makes a missing credential silent, and it has already cost a
  channel.** The mail credentials live in their own env file
  (`/opt/freehire/.env.notify`, not the `/opt/freehire/.env` every worker reads); the
  `remind` and `nudge` units did not load it, so email reminders soft-skipped from the day
  they shipped until 2026-09-01 while every run exited 0 with `failed=0`. 244 of them piled
  up across 43 people. The health signal for these workers is therefore **`soft_skips` in
  the run log, not the exit code**: a steady non-zero count against `delivered=0` is a dead
  channel, not an absence of recipients. See [deploy/AGENTS.md](../../deploy/AGENTS.md) for
  which units must read both files.
- **A blocked Telegram bot unlinks the chat; it does not fail the delivery.** Every 403 the
  Bot API answers a send with means the chat is permanently closed (blocked, deactivated,
  bot removed), and no retry reaches it. `telegramnotify.ErrChatUnreachable` carries that up;
  each engine's Telegram notifier translates it into its own `ErrRecipientGone`, and the
  runner deletes the user's `telegram_links` row and soft-skips. **Matched on the 403, not on
  the description text** — that text is prose Telegram may reword, and a rule keyed to
  "Forbidden: bot was blocked by the user" would stop firing silently. Unlinking rather than
  disabling the subscription is the point: blocking the bot is a fact about the USER, so one
  delete makes every telegram delivery for them — digest, reminder and nudge alike — read as
  "not linked" and soft-skip, while their subscriptions survive for whenever they relink.
  Before this, one blocked subscriber failed a digest per pass forever and kept
  `freehire-notify.service` in `failed`.
- **Matching is O(distinct queries), not O(subscribers).** `notify.Runner.Run` groups
  subscriptions sharing a saved-search query so the search index is hit once regardless of
  how many people subscribed to it. A per-subscription loop would multiply index load by
  subscriber count.
- **Avoid-skills is enforced as a per-subscriber post-filter, not folded into the shared search.**
  `Runner.match` batch-fetches every active subscriber's live `user_profiles.excluded_skills` once
  per pass (`Store.ListUserProfilesExcludedSkills`) and `matchQuery` skips a `(hit, subscription)`
  pair whose job carries a skill that subscriber currently avoids — evaluated against the *live*
  preference, not whatever `skills_exclude` (if any) got frozen into the saved search's own query
  string at creation time. Do not move this into the Meilisearch `Filter`: that would make the
  filter subscriber-specific and defeat the canonical-query grouping above, turning matching back
  into O(subscribers).
- **The dedup ledger's primary key is what makes matching idempotent.** MATCH records
  matched jobs, DELIVER leases them and marks them notified — so re-scanning recent jobs
  never delivers twice. Preserve the two-stage split; merging match and send loses the
  guarantee.
- **`internal/engage/nudge`'s dedup key adds an "episode key"** — the fact that must change before a
  re-notify is warranted (an application's `last_activity_at` for a follow-up nudge, a
  `stage_set` event's `occurred_at` for interview-prep) — alongside `(user, job, kind)`. This is
  what lets MATCH re-scan the same still-silent application every pass without re-pinging it: the
  episode key is unchanged, so the insert is a no-op against `application_nudges`' unique index.
  No snooze interval, no notified-count column. `internal/engage/reminder`/`internal/engage/notify` don't need
  this — a reminder fires once from a pre-scheduled `fire_at`, a subscription match is a distinct
  `(subscription, job)` pair already.
- **Every grouped message is bounded twice, and the two bounds are not the same number.**
  `Config.SnapshotCap` (200) is everything the group carries and is what the in-app
  notification records — what `/my/notifications/:id/jobs` renders. `notify.ListLimit`
  (10) is what a channel message itemizes; the "and N more" tail is the difference. All
  three engines carry both, and `ListLimit` is a single exported constant shared by all of
  them. They were one knob until 2026-08-21, which meant lowering the email's list length
  silently truncated the on-site page the email's own "view all" pointed at. Beyond
  `SnapshotCap` the excess is RELEASED back to the pending queue, never stamped delivered:
  an item marked delivered while appearing in no message is gone for good.
- **`Digest.Total` is `len(Jobs)`, and that is load-bearing.** A pass can claim more matches for
  one subscription than `SnapshotCap` allows; `deliverOne` calls `deferOverflow` to release the
  excess back to the pending queue BEFORE building the digest, so a later pass delivers it. Do
  not go back to truncating in `buildDigest` — that stamped the overflow notified while it
  appeared in no message and in no snapshot, dropping those postings from the alert for good. A
  claimed id whose job row was pruned is deliberately NOT deferred; it is stamped notified, or
  it would be re-claimed every pass forever.
- **A subscription digest is recorded BEFORE it is sent** (`RecordNotification` is `:one`), so
  the message can link to its own `/my/notifications/<id>/jobs`. A failed send withdraws the row
  (`DeleteNotification`) — best-effort: if that delete also fails it is logged, and one history
  row describes a digest nobody received. A failed *recording* is non-fatal in the other
  direction — the digest goes out with `NotificationID` zero and each channel's tail falls back
  to `/my/notifications`. `internal/engage/reminder` and `internal/engage/nudge` still record after delivery
  and discard the returned id.
- **`user_notifications.jobs` is one shape owned by one package.** `notify.SnapshotJob`
  (`{title, company, slug}`, migration 0091) is what all three engines write and what the
  single `/my/notifications/:id/jobs` page reads. A group of MORE than one fills `jobs`
  and leaves `public_slug` NULL; a group of one does the opposite. Three private copies of
  the shape would each be right until one of them changed.
- `DigestJob` deliberately carries **no internal job id** — only the public slug and URL.
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
- Salary fields are projected from enrichment; zero min/max or an empty currency means
  unknown, and the renderer omits the line rather than printing a zero.

## Note on naming

`internal/engage/telegramnotify` (outbound, notifications) is the sibling of `internal/ingest/telegram`
(inbound, channel crawling for vacancies). They are unrelated concerns that both talk to the
Bot API — check which one you're in.

## Limitations

- Delivery is cron-driven and run-once; there is no retry queue. A channel outage means that
  pass's digests are skipped, and the ledger redelivers them on the next pass only if they
  were never marked notified.
