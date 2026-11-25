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

It also stops being a *truncation* and becomes a *deferral*: `deferOverflow` hands
the excess back to the pending queue instead of letting `buildDigest` drop it, so
the ceiling bounds one delivery rather than bounding what a subscriber ever sees.
That is what lets `buildDigest` lose its `limit` parameter — with nothing left to
cut, `Total` is simply `len(Jobs)`.

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

1. `deferOverflow`, then build the digest
2. `RecordNotification` (now `:one`) → id
3. `digest.NotificationID = id`
4. `Send`
5. on send failure: `DeleteNotification(id)`, then the existing
   `RecordMatchDeliveryFailure` path
6. on success: `MarkMatchesNotified`, `MarkDigestSent` as before

Step 5 keeps an undelivered digest out of the history, which is what the old
ordering gave for free. The `add-notification-center` requirement ("record one row
for every successful delivery", "a recording failure SHALL NOT fail the delivery")
stays literally true: step 2 failing is logged and delivery continues with
`NotificationID` zero.

**The withdrawal is unconditional, including on ambiguous errors.** A `Send` that
times out may mean the mail went out and only the acknowledgement was lost, so
withdrawing can delete the row for a digest that was in fact delivered — its "view
all" link then 404s. Withdrawing only on errors that prove nothing was dispatched
(`ErrChannelNotConfigured`) sounds safer and is worse: the matches stay pending
under either reading, so every retry of a genuinely failed send would leave another
row, and a channel outage would fill the history with up to `MaxAttempts` rows per
digest nobody received — the exact failure this ordering exists to prevent. One
dead link on a rare ambiguous send beats five phantom entries on every real one.

**This is best-effort, not an invariant, and the spec says so.** A `Send` failure
followed by a `Delete` failure leaves one history row describing a digest nobody
received. It is logged. No reconciler is added for it: the sweep would have to
distinguish a phantom row from a legitimately recorded digest, and the only
distinguishing fact — that the send failed — is not written anywhere. Recording
it to make a sweep possible costs a column and a pass to remove a row that is,
at worst, one stale entry in a read-only list. Wrapping steps 2–5 in a transaction
would close the window exactly, at the cost of holding a Postgres transaction open
across an SES/Telegram round-trip for every digest in the batch, and of plumbing
transaction support into a `Store` interface that has never needed it. Neither is
worth it for a window this narrow with a consequence this small — but the guarantee
is stated as best-effort rather than "if and only if", because a reader who takes
the stronger reading will eventually be wrong.

`RecordNotification` is shared with `internal/reminder` and `internal/nudge`; both
become `_, err :=`. Adding a second, near-identical insert just to keep a `:exec`
signature would be worse than the two-character change at each call site.

### The link target

`<origin>/my/notifications/<id>/jobs`. The page exists (`notification-center-navigation`
already specifies it, down to keeping the History tab active), it fetches its own
data, and it is a valid bookmark on its own — it was built to be linked to. Nothing
new is needed on the web side.

## Risks / Trade-offs

**A subscription that claims more than 200 matches in one pass** — reachable by a
`daily` digest accumulating across deferred passes, and by `ClaimBatch` (500)
handing one subscription up to 500 ids. `deliverOne` calls `deferOverflow` first:
the freshest 200 are delivered and the rest are released back to pending for a
later pass, so `Total == len(Jobs)` always holds and the tail can never name a job
the linked page cannot show.

The trap this avoids is the pre-existing one. `buildDigest` used to truncate to
the cap while `MarkMatchesNotified` stamped the *whole* claimed set, so the
overflow left the alert having appeared in no message and no snapshot. At the old
cap of 20 a 500-match claim silently dropped 480 postings. The overflow is now
deferred rather than truncated, which is also why `buildDigest` no longer takes a
limit at all — a digest that could carry less than it counted is exactly the shape
that made the bug possible.

One id is deliberately exempt: a claimed match whose job row was pruned between
matching and delivery returns no row from `GetJobsForDigest`, so it cannot be
deferred by job — it stays in the notified set. Deferring it would re-claim it
every pass forever.

**A `daily` digest spreads across days when it overflows.** 500 matches deliver
200 today and the rest tomorrow, because `MarkDigestSent` stamps the day. Accepted:
delayed beats dropped, and a saved search matching 500 jobs a day is one that wants
narrowing more than it wants a bigger mail.

**Ten may prove too few for a `daily` digest**, where a bigger list is more
expected than in an instant alert. `ListLimit` is one constant and a
frequency-dependent limit is a one-line change if the feedback arrives; guessing at
it now is the overengineering this repo's working principles warn against.
