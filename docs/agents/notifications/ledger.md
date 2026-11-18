# Notifications: the dedup ledger and the delivery record

Part of [notifications](../notifications.md): what makes matching idempotent, how a digest is bounded and goes stale, and how a delivery is recorded.

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
