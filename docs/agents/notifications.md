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
| `internal/engage/webhooknotify` | Webhook channel (plain, unsigned HTTP POST to an account's own URL) — implements `notify.Notifier` for `notify` ONLY, see the bullet below | — |

## Daily inbox summary

`internal/engage/inboxdigest` runs inside `cmd/notify`, before saved-search matching,
so missing Meilisearch configuration does not disable inbox mail. It uses the same
SES transport and engagement mail pause. No extra timer or model call is introduced.

- A live Pro or Ultra plan is required to enable or deliver inbox digests. Expiry pauses
  delivery without erasing consent; disabling remains available on every tier. The
  activation service uses `plan.TierOf(...).Paid()`, and recipient selection and the
  final delivery claim independently check both entitlement columns. No AI allowance is consumed.
- `notification_settings.inbox_digest_enabled` defaults false. Enabling records an
  activation timestamp; older clients omitting the field preserve consent. Delivery
  requires the global activity switch, the email channel and `mail_recipient` eligibility.
- The existing daily `digest_time` and account timezone schedule it independently of
  saved-search frequency. Empty or invalid timezones fall back to UTC.
- Select classified incoming non-`other`, non-deleted mail received since activation,
  within seven days. Group by source and provider thread, with per-message fallback;
  show the newest message in each conversation and a link to `/my/inbox?message=<id>`.
  A visible later outgoing reply is evidence, not a claim that every request is resolved.
- Show ten conversations and count overflow; successful delivery retires the entire
  covered window, preventing an old backlog. A late-classified message superseded by
  a newer already-delivered message in its thread cannot resurrect an outdated request.
- A unique `(user_id, local_day)` claim precedes dispatch and rechecks live consent,
  address, timezone and schedule. Empty snapshots claim nothing. An uncertain send or
  failed acknowledgement retains the claim for that day; unconfirmed messages can be
  retried on a later day. A definite pre-dispatch SES pause releases its claim. Sending
  and the ledger are not one transaction, so cross-day replay after a crash is possible.
- Generated summaries carry `Auto-Submitted: auto-generated` and the shared
  `mailmeta.NotificationHeader` marker. Gmail and hosted ingest recognize the exact
  inbox-digest kind and skip it, preventing the summary from becoming its own next input.

Apply migrations 0265 and 0266 before deploying. Roll out the Gmail reader guard and
restart the existing mail-ingest daemon before enabling the new notify behavior. Preview: the `inbox-digest` sample
in `design-system/static/email-previews/`; settings: `/my/notifications/settings`.

## Always true

- **Every subscription with something pending gets served in a pass — the claim is capped
  PER SUBSCRIPTION, not globally.** `ClaimSubscriptionMatches` takes at most `SnapshotCap`
  rows per subscription through a `LATERAL`; `ClaimBatch` is a backstop above
  `SnapshotCap x active subscriptions`, not the working bound. Set it low and the cut
  lands on whichever subscriptions the scan reached last, which is the failure this
  replaced: the claim used to be one flat `ORDER BY subscription_id, matched_at LIMIT
  batch_size`, which reads as "oldest first" and is really "lowest subscription id
  first". Measured on prod 2026-09-04, reported as "I get no Telegram notifications":

  ```
  subscription  26 ->    1 118 pending
  subscription  95 ->  248 147   <- saved search with an EMPTY query
  subscription  97 ->  104 069
  the reporter's   ->    8 324    attempts = 0
  queue total      -> 1 141 757
  ```

  Nothing failed. `notify` ran every five minutes and logged `delivered=1 failed=0` —
  one digest, to the same low id, pass after pass, for weeks. Every subscription above it
  had `attempts = 0` since the day it was created: not retried, not dead-lettered, never
  in a batch. **`failed=0` is not evidence that delivery works**; `delivered` far below
  the number of subscriptions with pending matches is what says it does not.

- **A subscription must carry a filter.** An empty saved search is legitimate — it is the
  "show all" view, and `internal/search/savedsearch` says so. SUBSCRIBING to one asks to
  be notified about every posting in the catalogue, which is never what anyone means, and
  is where the 248k above came from. `subscription.Service.Create` refuses it with
  `ErrUnfilteredSearch` (400). The guard is at subscribe time, not at save time, because
  that is where the meaning changes.

- **The company watchlist alert is a saved search, not a second engine.** A person's list
  (`internal/engage/companywatch`, table `company_watchlist`, up to `MaxEntries` = 5000) sends
  nothing itself; its alert is a saved search carrying `watchlist=true`, subscribed to like any
  other, so channels, quiet hours and digest sections come for free. The same query string
  means a different set of companies to each person, so `cmd/notify` groups such a query
  **per subscriber** (`matchGroup`) instead of per query string, and narrows its search with
  `company_slug IN <their list>` read at match time (`ListWatchlistSlugsForUsers`) — editing
  the list changes the next search with no re-subscribe. **An empty list matches nothing**: the
  group runs no search at all, because dropping the filter would send the whole catalogue. The
  param counts as a narrowing for `search.Narrows`, so the subscribe gate admits it. The list
  keeps a slug a merge retired and follows it through `company_slug_aliases` on every read,
  and has no foreign key to `companies` (that table is derived from jobs and loses an employer
  the day its last posting closes — the day a watcher most wants to hear about it).

- **A subscription must carry a query the search can RUN, and stays that way.** Create
  and resume (`subscription.Service.SetActive(true)`) share one gate (`admit`): the
  query parses (`search.ParseQuery` — e.g. at most 5 OR alternatives), any nonempty
  `visa_sponsorship` value is a boolean, the query narrows, and the
  channel can reach the account. Saved searches check only that the query parses, on
  create and edit — an unfiltered one is a legitimate "show all" view; narrowing and
  the channel are a subscription's gates. What predates a rule — a query saved before it existed, or one a tightened rule
  later invalidates — is caught by `cmd/notify` itself: `pauseUnrunnable` pauses every
  subscription on a query it cannot parse or whose invalid `visa_sponsorship` value
  would now be dropped, widening the old alert. It does not search it (`paused=` in the run
  log), and only while the saved search still holds the query it judged, so an owner
  who fixed it meanwhile keeps the alert on. Measured 2026-11-05, one such subscription had failed every pass for a week
  while its owner received nothing. Paused, not deleted, so the owner can fix the
  search and resume.

- **A non-positive per-subscription cap claims nothing and reports success.** It becomes
  `LIMIT 0` inside the `LATERAL`: no rows, no error, the worker quietly idle. It is
  floored at the call site for the same reason `SEARCH_DRAIN_BATCH_SIZE` is — a silent
  no-op is worse than a crash.

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
- **A saved-search digest has three frequencies, and the default is an hourly batch.**
  `notification_settings.digest_frequency` is `interval` (at most one message per
  person and channel every `digest_interval_hours`, 1-23), `instant` (every pass that finds
  something) or `daily` (once a local day at `digest_time`). `interval` at one hour is the
  column default, what an account with NO settings row reads as, and since migration 0233
  what every stored `instant` was moved to — measured 2026-11-04, `instant` meant a p90 of
  ~32 digests a day and 141 for the busiest account, on 96% of subscribers, most of whom
  never opened the setting. **The interval gate is in `ClaimSubscriptionMatches`, not in
  `deliverOne`**: a subscription that is not due is not claimed at all, because claiming
  and releasing ~23k matches an hour on every five-minute pass is write churn for
  nothing. That SQL predicate is the only definition of "interval elapsed"; its defaults
  (`'interval'`, `1`) must agree with `GetSubscriptionForDelivery`'s `COALESCE`, the
  column defaults and `notify.DefaultIntervalHours`. `interval` defers to quiet hours like
  `instant` (it names no hour the person chose; `daily` does and is exempt), and both
  `interval` and `daily` stamp `subscriptions.last_digest_sent_at` after a send. The
  interval is measured from that stamp, so on a five-minute timer an hourly digest lands
  every 60-65 minutes rather than on the hour.
- **Quiet hours, the `daily` time and a missing recipient are decided BEFORE the claim.**
  `deliver` lists the subscriptions with a pending match (`ListDeliveryCandidates`, the same
  columns as `GetSubscriptionForDelivery` plus the person's last send, so one converts to the
  other), asks `deliveryHold` (`deliverywindow` + `recipient`) which can go out now, and passes
  only those ids to `ClaimSubscriptionMatches`. Before, they were claimed and released again on
  every pass (~8.5M `subscription_matches` updates in a day, 2026-11-11 audit). `deliverMessage`
  asks `deliveryHold` again after the lease for the races in between, so a held message still
  releases rather than sends. The SQL interval gate and the email opt-out stay in the claim.
- **One message per person and channel, carrying a digest per due subscription.**
  `deliver` groups the claim by `(user_id, channel)` (`ClaimSubscriptionMatches` returns
  both) and sends one `notify.Message`: one mail with a section per alert, one Telegram
  message, one push counting the alerts. A message of one digest renders exactly as a
  digest always did. Before this the cadence was per SUBSCRIPTION, so a person with seven
  alerts got seven mails an hour — measured 2026-11-05, 15 users over 24 a day, the top
  one at 59 — and a bulk rewrite of 578 profile alerts on 2026-11-03 put 580 people over
  24 in one day. The interval gate therefore reads the person's NEWEST
  `last_digest_sent_at` on the channel, and a sent message stamps every subscription it
  carried; a new subscription waits for the person's next message instead of firing on
  its own. **A webhook is the exception**: its receiver is a program reading one saved
  search per payload, so it stays one subscription per message and keeps its own stamp.
  The notification centre still records one row per saved search, so each section's
  "View all" opens its own page. `freehire_notify_users_over_daily_cap_24h` (cmd/queue-metrics)
  is the gauge that says whether this holds.
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
  channel, not an absence of recipients. See `freehire-ops`' `provision/host2/AGENTS.md` for
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
- **A query is searched only when its digest could go out** (`notify.searchDue`). Grouping
  buys little in practice — measured 2026-11-05, 2,451 active subscriptions carried 2,332
  distinct queries — so the real cost was the cadence: every five-minute pass ran all
  ~2,100 of them while an hourly digest can use one search an hour, and Meilisearch was
  the busiest process on the host (~4 cores, ~530 MB/s read). A person's subscriptions
  (user and channel; a webhook alone) are searched together: `instant` every pass;
  `interval` once the interval since the last message has elapsed, outside quiet hours,
  and then once per interval while nothing is found; `daily` when `DigestDue` says the
  day's digest is due, hourly while it stays due. The result is that a posting is found
  no later than one interval after it appears and sent in that same pass — the latency
  the interval already promised. `subscriptions.last_searched_at` (migration 0239) holds
  when a subscription was last searched, stamped with the pass's START, and a search
  after a gap **pages back** to it (less `matchLag`, an hour, for the drain's indexing
  delay; at most `maxMatchPages`), so the gap loses nothing a five-minute scan would
  have recorded. A never-searched subscription is always due and reads one page. The
  run log's `skipped_queries` and `searches` (index requests, counting pages) are what
  measure it. **Carry-over sends are the one seam**: a message released earlier (a
  failed send, a relinked chat) can go out on a pass that did not search, and what was
  indexed since the last search then waits for the next due pass.
- **A saved search's minimum skill match is applied per subscriber, after the search.**
  `min_skill_match` is a web filter the search never reads: the percent depends on the reader, so
  `Runner.match` computes it per (hit, subscription) pair against the subscriber's live
  `user_profiles.skills`, with the same formula as the job list (`computeClientMatch`). It can
  only thin the pages the search read: on a broad query, jobs clearing the threshold but beyond
  those pages are not reached. A query carrying only this param does not narrow the catalogue, so
  a subscription to it is refused like any other unfiltered search (`search.Narrows`).
- **Avoid-skills is enforced as a per-subscriber post-filter, not folded into the shared search.**
  `Runner.match` batch-fetches every active subscriber's live `user_profiles.excluded_skills` once
  per pass (`Store.ListUserProfileSkillSets`) and `matchQuery` skips a `(hit, subscription)`
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
  silently truncated the on-site page the email's own "view all" pointed at.
- **A subscription digest carries the NEWEST matches and COUNTS the rest — it never
  re-queues them.** The claim takes `SnapshotCap` per subscription, newest first. When it
  comes back full, `CountCoveredMatches` measures what is still pending behind its oldest
  row, the digest's `Total` includes that count, and once the digest is delivered
  `MarkCoveredMatchesNotified` stamps the same set (same predicate, same `(matched_at,
  job_id)` cutoff — `matched_at` alone ties across a whole match batch). Such a digest is
  `Overflowed()`, and its "and N more" tail links to the saved search's live results
  (`/jobs?<query>`), because the notification's own page lists only the snapshot it
  carries. The excess used to be RELEASED back to pending and the claim read oldest first,
  so a `daily` subscription matching 245–1000 jobs a day sent one digest a day of its 200
  OLDEST matches and never caught up — measured on prod 2026-10-22: 33k pending on daily
  email subscriptions, 10.5k older than 7 days, subscription 256 being mailed matches
  from three weeks earlier. A failed send retires nothing it covered; a failed count makes
  the digest announce only what it carries and retire only that. A claimed id whose job
  row was pruned stays in the claim and is stamped notified, or it would be re-claimed
  every pass forever.
- **A match records the query revision it answered.** `saved_searches.query_revision` rises
  by one only when `UpdateSavedSearch` changes the query text (a rename leaves it), and
  `subscription_matches.query_revision` stores the revision a match was found under (migration
  0243). `RecordSubscriptionMatches` takes it per pair from the `ListActiveSubscriptions` row
  and refuses a pair whose search has moved on, so a pass that read the old query cannot
  record after the edit; a still-pending row of an older revision is reset on conflict, or a
  job both queries match would stay blocked. Claim, `CountCoveredMatches`,
  `MarkCoveredMatchesNotified` and the pending metrics read only the current revision, and
  `ExpireStaleSubscriptionMatches` retires unleased superseded rows. This covers a leased match
  released or lapsing after an edit, which the edit's DELETE of unleased pending rows cannot.
  It is a counter and not `updated_at` (a rename would strand valid matches) and not a clock
  comparison (two in-flight statements cannot be ordered by timestamp).
- **A match goes STALE, undelivered, after `Config.FreshnessWindow` (3 days) or once its
  job closes.** `ExpireStaleSubscriptionMatches` runs first in every delivery stage, in
  bounded batches until one comes back short, and stamps `expired_at` (migration 0193) —
  a terminal state of its own, because a stale match was neither shown to anyone
  (`notified_at`) nor attempted (`failed_at`, the dead-letter queue an operator reads), and
  deleting it would let the match stage record it again as new. This is what bounds the
  backlog of a subscription nobody can receive — push with no device, an unlinked
  Telegram chat, a disabled webhook: on 2026-10-22 those held 27k, 7.6k and 4.6k pending
  matches reaching back to 2026-09-02, roughly half for closed jobs, all of which a
  relinked user would have been sent oldest first. It also bounds
  `NotifyBacklogMetrics`' oldest-pending age at the window: an age above it means expiry
  has stopped. The metric is split by a `deliverable` label computed by the SQL twin of
  `recipient()` — the undeliverable half sits AT the window by construction, so only the
  deliverable half is worth alerting on. Change the rule in one place and change it in the
  other. A failed expiry fails the delivery stage rather than claiming around it.
- **A subscription digest is recorded BEFORE it is sent** (`RecordNotification` is `:one`), so
  the message can link to its own `/my/notifications/<id>/jobs`. A failed send withdraws the row
  (`DeleteNotification`) — best-effort: if that delete also fails it is logged, and one history
  row describes a digest nobody received. A failed *recording* is non-fatal in the other
  direction — the digest goes out with `NotificationID` zero and each channel's tail falls back
  to `/my/notifications`. `internal/engage/reminder` and `internal/engage/nudge` still record after delivery
  and discard the returned id.
- **The history holds one row per EVENT, not per channel — and that is enforced by a key,
  not by the shape of the loop.** `subscriptions` is keyed `(saved_search_id, channel)`, so
  `deliverOne` runs once per enabled channel; the row it writes was always meant to be
  channel-agnostic (migration 0090 says so in its opening line) and for three years was not.
  One saved search with Telegram, email and push left three indistinguishable
  `My profile — 1 new job` entries for one match (freehire#3020 — measured on prod
  2026-09-18: 322 of 27,207 digest rows were a redundant second copy, 1.2% overall and 100%
  of the history of anyone with a second channel). `notify.digestDedupKey` names the event
  — the saved search plus the sorted set of job ids, hashed — and migration 0174's partial
  unique index on `(user_id, dedup_key)` is what makes the first channel's write win and
  every later one return no row. **Only the channel that CREATED the row may withdraw it**:
  `recordNotification` returns that as `created`, and without it a third channel's failed
  send would delete the history of the two digests that did arrive. The engines that already
  record once per event pass a NULL key and are outside the index entirely.
- **A digest that WAS delivered is recorded even if the pre-send write came back with
  nothing**, so `deliverOne` writes the row in two places and that is deliberate. The
  pre-send write is what gives the message its own id to link at; when it fails — an
  ordinary database hiccup, or (needing a second notify process) a conflict with a row
  withdrawn before this channel could read it — the digest still goes out, and until
  freehire#3020's follow-up it went out leaving nothing in the history at all. The
  post-send write is safe to attempt blindly because the dedup key makes a redundant
  insert a no-op, so a first write that in fact succeeded and only lost its answer cannot
  become a second row. It is the one place the ordering may follow the send: the id is
  no longer needed for anything, the message is gone and its tail already fell back to
  the generic destination. **One ordering is still open and is written down rather than
  fixed**: across two overlapping passes a joiner can send successfully and the creator
  then withdraw the shared row underneath it. The joiner read a live id, so it has
  nothing to notice; closing it needs per-event delivery state this engine does not keep,
  and systemd will not stack a `Type=oneshot` unit on itself, so reaching it takes a
  hand-run pass overlapping the timer's.
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
  were never marked notified — and only while they are fresh: an outage longer than the
  freshness window loses what it held back, by design.

## Pausing outbound mail

Outbound mail workers can be held by class (transactional, engagement, lifecycle) or all at
once with one Redis key, and a worker stops sending for the rest of its run once SES refuses the
whole account. The keys, the class table, the fail-open note, the resume order
(transactional, then engagement, then lifecycle) and the breaker are in
[internal/platform/worker/AGENTS.md](../../internal/platform/worker/AGENTS.md#holding-outbound-mail-by-class).
A hold sends nothing and never bypasses unsubscribe or consent rules; the API server's own
mail is never held.

## Engagement tracking

Campaigns (`cmd/broadcast`) and the saved-search digest (`cmd/notify`) can be measured for delivery
and clicks; nothing
else can. A message is tracked only when it names a `Campaign` and its client was given a
configuration set (`WithEngagementTracking`, from `SES_ENGAGEMENT_CONFIGURATION_SET`), so account,
security and application mail cannot pick up an open pixel by accident.

- **The flow:** SES configuration set → SNS → SQS (`EMAIL_EVENTS_QUEUE_URL`) → `cmd/email-events`,
  the same SNS → SQS shape as inbound mail, with no public endpoint to authenticate.
- **No person in the ledger.** `email_events` holds a message id, campaign, variant, type and time,
  first sighting per message and type. The consumer does not decode the recipient, IP or link.
- **Retention:** 90 days, then `email_event_daily` counts only. Events for mail sent longer ago
  than that are dropped, since the sighting that would deduplicate them may be gone.
- **A set SES does not know is caught before sending.** `CheckEngagementTracking` needs
  `ses:GetConfigurationSet`. `cmd/broadcast` refuses to start, because recorded failures would
  spend the audience; `cmd/notify` falls back to untracked mail, because a digest is retried.
- **Clicks only, no open pixel.** The configuration set publishes no Open event, which is what
  keeps SES from adding the pixel. Opens would overcount anyway: Apple Mail Privacy Protection and
  security scanners fetch it without a person reading anything. The owner chose this on 2026-11-05.
  Scanners can follow links too, so a click within seconds of delivery deserves suspicion.
- **Bounces and complaints on every other mail.** The sending identity (`mail.freehire.me`,
  us-east-1) has `freehire-feedback` as its default configuration set, which publishes only
  BOUNCE and COMPLAINT to the same topic. Every mail carries a `group` tag, and the consumer
  records such an event under `campaign = 'group:<group>'` (`group:essential`, `group:news`, ...),
  so a complaint spike names the kind of mail behind it. A message naming
  `freehire-engagement` overrides the default, so campaign mail is not counted twice.
- **Comparing two send times:** run `./broadcast -campaign <name> -half a` at one time and
  `-half b` at the other. The sides are hashed with the campaign name and stable across runs. Then:

```sql
SELECT variant, type, count(*) FROM email_events WHERE campaign = '<name>' GROUP BY 1, 2 ORDER BY 1, 2;
```
