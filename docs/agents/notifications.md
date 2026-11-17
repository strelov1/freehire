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
| `internal/engage/webhooknotify` | Webhook channel (plain, unsigned HTTP POST to an account's own URL) — implements `notify.Notifier` for `notify` ONLY, see [notifications/channels.md](notifications/channels.md) | — |

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

Needs migration 0269 (`release.sh` applies it). Roll out the Gmail reader guard and
restart the existing mail-ingest daemon before enabling the new notify behavior. Preview: the `inbox-digest` sample
in `design-system/static/email-previews/`; settings: `/my/notifications/settings`.

## Weekly application summary

`internal/engage/applicationdigest` runs in `cmd/notify` independently of search,
using the same SES sender and activity unsubscribe. `application_digest_enabled`
is a separate, explicit opt-in, defaulting false, with the same paid entitlement
policy as the inbox summary. Missing update fields preserve existing consent.

Delivery is due Monday at the existing daily `digest_time` in the account timezone.
The summary reports seven-day application events and current application stages.
It offers up to five follow-ups using the tracking board's silence rules, shows
previous chase dates, and never sends mail to an employer. Unconfirmed mail
suggestions cannot turn into a claim that the employer has not responded.

Empty summaries do not claim a week. A unique user/local-week claim rechecks live
recipient eligibility and consent before dispatch. Uncertain dispatch retains the
claim; a definite sending pause releases it. Both inbound mail readers recognize
the `application-digest` generated notification marker to prevent summary loops.
Apply migration 0279 before the new readers, and deploy the Gmail guard and restart
mail-ingest before enabling delivery, as with the inbox summary.

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
- **A saved-search digest has three frequencies; Free is always `daily`.**
  `notification_settings.digest_frequency` is `interval` (at most one message per
  person and channel every `digest_interval_hours`, 1-23), `instant` (every pass that finds
  something) or `daily` (once a local day at `digest_time`). `daily` at 09:00 is the column
  default (migration 0246) and what an account with NO settings row reads as; before that
  the default was an hourly `interval`, and migration 0233 moved every stored `instant` to
  it — measured 2026-11-04, `instant` meant a p90 of ~32 digests a day and 141 for the
  busiest account, on 96% of subscribers, most of whom never opened the setting.
  **Anything faster than `daily` is a Pro/Ultra benefit.** The stored value is a
  PREFERENCE; the schedule that takes effect is computed where it is READ, by the same SQL
  expression in `ListActiveSubscriptions`, `ListDeliveryCandidates`,
  `GetSubscriptionForDelivery` and the interval gate of `ClaimSubscriptionMatches`: an
  account whose `users.pro_until` and `ultra_until` (any source: Stripe, RevenueCat,
  granted) are both past reads as `daily` at its stored `digest_time` and timezone, so an
  expired subscription or a legacy `instant` never buys speed, and nothing else (`searchDue`,
  the pre-claim `deliveryHold`, the per-person daily gate) needs to know. A renewal
  restores the stored preference. The **webhook** channel is exempt: it is a program's
  endpoint, not a person's digest, and keeps the stored value. The commercial API licence
  unlocks nothing here. On the settings write (`reminder.Service.UpdateSettings`) only a
  CHANGE to a faster frequency (or a changed interval) is refused for Free, with `402`
  (`ErrFasterAlertsNeedPaidPlan`): the page sends the whole rule back, so unrelated edits
  and opt-outs keep working with an old `instant`, and the stored preference is left
  alone. `GET` returns the stored preference plus the response-only
  `faster_alerts_available`; the web page and the MCP `delivery_schedule` show the schedule
  that takes effect. No search quota is attached: search itself stays free. Paid `instant`
  means "the next pass that finds something", not realtime: the worker runs on a timer.
  **The interval gate is in `ClaimSubscriptionMatches`, not in
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
- **Channels — the `Notifier` implementations, push, the channel vocabulary, soft-skips and a blocked Telegram bot — are in [notifications/channels.md](notifications/channels.md).**
- **Matching — one search per distinct query, searched only when its digest could go out, and the per-subscriber minimum skill match and avoid-skills post-filters — is in [notifications/matching.md](notifications/matching.md).**
- **Dedup, digest bounds, freshness and the delivery record — the ledger key, the episode key, query revisions, record-before-send and the one-row-per-event history — are in [notifications/ledger.md](notifications/ledger.md).**
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
