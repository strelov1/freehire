## Why

A subscription digest that matched 67 jobs arrives as an email listing 20 of them.
Twenty rows is not a notification, it is a page — nobody reads to the bottom, and
the mail's one job (tell me something new happened, take me to it) is buried.

The tail under those 20 rows is a button reading "View all — 47 more", and it does
not go to the other 47. It goes to `/my/notifications` — the notification section's
landing page. The user asked for all of them and got the alert list.

Both are the same root cause: one knob, `Config.DigestCap` (20), decides two
unrelated things. It caps how many jobs a *channel message* lists, and — because
`digestJobsSnapshot` reads the same already-capped `Digest.Jobs` — it also caps how
many jobs are written into the in-app notification's `jobs` snapshot. Lowering it to
make the mail shorter would silently shrink the on-site list by the same amount, so
"view all" would still be a lie, just a smaller one.

## What Changes

- **Split the one cap into two.** `Digest.Jobs` carries the full match set (bounded
  by a generous `Config.SnapshotCap`) and is what the in-app snapshot records.
  A new `Digest.Listed()` returns the first `notify.ListLimit` (10) — the jobs a
  channel message renders. `Digest.Total` keeps its meaning: the true count.
- **The digest learns its own notification id.** `RecordNotification` becomes
  `:one`, `deliverOne` records the in-app notification *before* sending, and the
  returned id travels on `Digest.NotificationID`. Each channel builds
  `<origin>/my/notifications/<id>/jobs` from it — the page that already renders a
  digest's full matched-job snapshot.
- **A failed send removes the record it created**, so the notification center
  still holds a row if and only if a digest was delivered — the guarantee the
  current record-after-send ordering provides. A failed *recording* does not block
  the send: the digest goes out with no id and the tail falls back to
  `/my/notifications`, exactly as today.
- **Email** lists 10 jobs and points its "and N more" button at the new URL.
  **Telegram**'s `+ N more` tail becomes a link to the same place; today it is bare
  text with nowhere to go.

**Not changing the push channel.** A push carries a title and a one-line body, never
a listing, so neither cap nor tail applies to it.

**Not changing the count in the subject.** "67 new jobs for …" stays — that number
is the news. Only the body is bounded.

## Capabilities

### Modified Capabilities

- `filter-subscriptions`: the delivery pass records the in-app notification before
  sending so the digest can carry its id, and removes it if the send fails; the
  digest's rendered listing is bounded separately from its recorded snapshot.
- `email-notify`: the digest email lists at most ten jobs, and its "and N more" tail
  links to the notification's own matched-jobs page.
- `telegram-notify`: the digest message's "+ N more" tail links to the same page.

## Impact

**Go:**
- `internal/notify/notify.go` — `Config.SnapshotCap` replaces `DigestCap`,
  `ListLimit` constant, `Digest.NotificationID`, `Digest.Listed()`.
- `internal/notify/deliver.go` — `buildDigest` caps at `SnapshotCap`; record →
  set id → send → delete-on-failure ordering.
- `internal/notify/push.go` — `digestJobsSnapshot` unchanged in behaviour, but now
  sees the full set; `renderDigest` untouched.
- `internal/emailnotify/notifier.go` — renders `d.Listed()`, new tail URL.
- `internal/telegramnotify/notifier.go` — renders `d.Listed()`, linked tail.
- `internal/db/queries/notifications.sql` — `RecordNotification` returns `id`; new
  `DeleteNotification`. `internal/reminder` and `internal/nudge` call sites absorb
  the extra return value.

**Web:** none. `/my/notifications/:id/jobs` already exists and already renders the
snapshot; it just gets a fuller one.

**Ops:** no migration, no backfill. Digests delivered before this change keep their
20-job snapshot; there is nothing to re-derive.
