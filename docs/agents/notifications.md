# Notifications

Two independent delivery use cases share one channel abstraction:

| Package | Use case | Worker |
|---|---|---|
| `internal/notify` | Filter subscriptions — new jobs matching a saved search | `cmd/notify` |
| `internal/reminder` | Saved-job nudges — come back before the vacancy goes stale | `cmd/remind` |
| `internal/emailnotify` | Email channel (SES) — implements `notify.Notifier` (the reminder-side email transport lives in `internal/reminder/transports.go`) | — |
| `internal/telegramnotify` | Telegram channel (Bot API, deep-link token) | — |

## Always true

- **A new channel is a new `Notifier` implementation — but there are TWO of them.** Each engine
  depends only on its own `Notifier` interface plus a `Router` (a `map[channel]Notifier`); the
  two interfaces share a signature and differ in payload, and `notify`/`reminder` each carry
  their own `Router`, `ErrChannelNotConfigured` and `recipient`. So adding webhooks is a digest
  notifier, a reminder transport in `internal/reminder/transports.go`, a case in BOTH `recipient`
  functions and a wire-up in both `cmd` mains. That is the real cost; the earlier claim that it
  "means adding a package, not touching `notify` or `reminder`" was not true. Collapsing the pair
  is deliberately deferred until a third channel actually lands and shows which half generalises.
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
- `Digest.Jobs` is capped to the configured digest size while `Digest.Total` carries the true
  count, so a renderer can show the "and N more" tail. Don't derive the count from `len(Jobs)`.
- `DigestJob` deliberately carries **no internal job id** — only the public slug and URL.
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
