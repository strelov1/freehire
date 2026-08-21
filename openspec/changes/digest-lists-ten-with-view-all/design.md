## Context

`internal/notify` builds one `Digest` per subscription per pass and hands it to a
channel `Notifier`. `buildDigest` (`deliver.go:164`) already truncates:

```go
d := Digest{SavedSearchName: name, Total: len(jobs)}
for i, j := range jobs {
    if i >= limit { break }   // limit = cfg.DigestCap, 20
    ...
}
```

So `Digest.Total` is honest and `Digest.Jobs` is short. Both channel renderers
compute their overflow tail the same way — `more := d.Total - shown` — which is why
the "47 more" figure in the mail is correct even though the list is not complete.

The truncation has a second consumer nobody reading the mail would guess at.
`deliverOne` calls `digestJobsSnapshot(digest)` (`push.go:94`), which marshals
`d.Jobs` into the `user_notifications.jobs` column, and that column is what
`/my/notifications/:id/jobs` renders. The in-app "which jobs were these" page is
therefore truncated by the *email's* cap. One number, two jobs.

## Goals / Non-Goals

**Goals**

- A digest message lists ten jobs at most.
- The overflow tail leads to the rest of them, not to the settings section.
- The in-app snapshot stops being collateral damage of the message cap.

**Non-Goals**

- Reworking the notification-center page. It already renders whatever snapshot it
  is given.
- A public, unauthenticated permalink for a digest. The matched set is one user's,
  it lives under `/my`, and the page 404s for anyone else. A signed-out reader
  meeting the login form is the correct outcome, not a bug to design around.
- Live re-running the saved search from the mail. A digest names the jobs that were
  new *at delivery time*; a fresh search drifts (listings close, new ones appear),
  so "view all" would show a different set than the one being announced.

## Decisions

### One cap becomes two, and only one of them is configurable

`Config.DigestCap` is renamed `Config.SnapshotCap` and keeps a generous value
(200). It is not a display concern any more — it is the ceiling on how much of a
match set we are willing to serialize into a jsonb column, and 200 both covers the
realistic case by a wide margin (`Config.MatchLimit` is 200: one query cannot match
more than that in a pass) and keeps a pathological accumulation — a `daily` digest
deferred across many passes — from writing an unbounded document.

The message-side limit is a package constant, not config:

```go
// ListLimit is how many jobs a channel message itemizes...
const ListLimit = 10

func (d Digest) Listed() []DigestJob
```

Config would have to reach `internal/emailnotify` and `internal/telegramnotify`,
which today take only a base URL and a transport. A constant keeps the two channels
consistent with each other for free — which matters, because the tail's count
(`Total - len(Listed())`) has to agree with the list above it.

**Telegram keeps its own second cap.** Its render loop stops early when the next job
line would push the message past the 4096 UTF-16 unit limit, and that must stay: it
is the guard against a deterministic Telegram rejection that dead-letters the whole
batch. Ten lines will effectively never reach that limit, so the loop just becomes
the belt to `Listed()`'s braces — it now runs over `d.Listed()` instead of `d.Jobs`.

### The digest carries the notification id, not a finished URL

`Digest.NotificationID int64`, zero when unknown. Each channel builds its own URL,
because each already owns its base origin and its own UTM tag (`?utm_source=email`
vs `utm_source=telegram`). A pre-rendered URL on the digest would have to pick one.

Zero is a real state and both renderers handle it: the tail falls back to
`/my/notifications`, which is where it points today. That keeps a recording failure
a degradation of the link, never of the mail.

### Record before send, undo on failure

`deliverOne` currently sends, marks notified, then records. The id cannot be known
before the send under that order, so the sequence becomes:

1. build the digest
2. `RecordNotification` (now `:one`) → id
3. `digest.NotificationID = id`
4. `Send`
5. on send failure: `DeleteNotification(id)`, then the existing
   `RecordMatchDeliveryFailure` path
6. on success: `MarkMatchesNotified`, `MarkDigestSent` as before

Step 5 is what preserves the guarantee the old ordering gave for free — a
`user_notifications` row exists if and only if a digest went out. The
`add-notification-center` requirement ("record one row for every successful
delivery", "a recording failure SHALL NOT fail the delivery") stays literally true:
step 2 failing is logged and delivery continues with `NotificationID` zero, and step
5 keeps an undelivered digest out of the history.

The residual window is a `Send` failure followed by a `Delete` failure, which leaves
one phantom history row. It is logged, and it is strictly better than the
alternative reading of the same window — dropping a delivery that already succeeded.
Wrapping steps 2–5 in a transaction would close it exactly, at the cost of holding a
Postgres transaction open across an SES/Telegram round-trip for every digest in the
batch, and of plumbing transaction support into a `Store` interface that has never
needed it. Not worth it for a window this narrow.

`RecordNotification` is shared with `internal/reminder` and `internal/nudge`; both
become `_, err :=`. Adding a second, near-identical insert just to keep a `:exec`
signature would be worse than the two-character change at each call site.

### The link target

`<origin>/my/notifications/<id>/jobs`. The page exists (`notification-center-navigation`
already specifies it, down to keeping the History tab active), it fetches its own
data, and it is a valid bookmark on its own — it was built to be linked to. Nothing
new is needed on the web side.

## Risks / Trade-offs

**A subscription that matches more than 200 jobs in one delivery.** The mail says
"N more", the page shows 200. The count stays honest — `Total` is still the true
figure — but the page under-delivers on "all". Accepted: `MatchLimit` makes this
reachable only by a deferred `daily` digest accumulating across passes, and the
alternative is an unbounded jsonb document per notification.

**Ten may prove too few for a `daily` digest**, where a bigger list is more
expected than in an instant alert. `ListLimit` is one constant and a
frequency-dependent limit is a one-line change if the feedback arrives; guessing at
it now is the overengineering this repo's working principles warn against.
