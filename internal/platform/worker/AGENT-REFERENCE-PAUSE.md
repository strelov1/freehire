# Worker conventions: pausing workers and holding mail

Part of the [worker conventions](AGENT-REFERENCE.md): the pause switch, holding outbound mail by class, and the SES circuit breaker.

## The pause switch

`Main` consults Redis before calling `run`, so a single command sheds load from the whole
cron fleet. It refuses to START a run; a run already in flight is never interrupted, and the
heaviest ingest boards take ~50 minutes to drain.

```bash
redis-cli SET freehire:pause:all 1 EX 10800     # hold everything for three hours
redis-cli SET freehire:pause:ingest 1 EX 10800  # hold one binary
redis-cli DEL freehire:pause:all                # lift early

FREEHIRE_IGNORE_PAUSE=1 ./backfill-derive       # run one thing against the held fleet
```

- **Presence is the signal**; the value is ignored, so the key can carry a note to whoever
  finds it. `<binary>` is `filepath.Base(os.Args[0])` — the same string as the `job` metric
  label, so the key you type matches the label you read.
- **The key names the binary, never the board.** One `freehire:pause:ingest` holds all ~140
  ingest timers. Holding a single board is that board's timer's job.
- **`EX` is a convention, not enforced.** A switch that could refuse to be set would fail at
  the moment it is most needed; the forgotten-switch case is handled by visibility instead.
- **`FREEHIRE_IGNORE_PAUSE` is parsed as a boolean**, unlike the keys — an operator who wrote
  `=0` means the opposite of a bypass. systemd timer units do not carry it, so it admits only
  a hand-started run.
- **It fails open.** An unreachable Redis, a malformed URL, or a reply slower than 250ms logs
  one line and the worker runs. A facility for shedding load must never become the reason the
  catalogue stops updating.
- **The budget is on the go-redis client, not only on the context.** go-redis applies its own
  5s `DialTimeout` and retries three times with backoff, so a `context.WithTimeout` alone lets
  a silent Redis stall every worker start for far longer than the deadline suggests.
- **`EXISTS` is called with both keys at once, which is a multi-key command.** Correct against
  the single Redis this runs on; on a cluster the two keys hash to different slots and the
  reply is a CROSSSLOT error, which fails open — meaning the switch would silently never hold.
  Split the lookup before moving Redis to a cluster.
- **`Main(run, extraKeys...)` takes further keys that hold this worker**, checked in the same
  `EXISTS`. This package knows nothing about what they mean: the outbound-mail block names them
  (next section). `AnyHeld` is the same check (bypass, 250ms budget, fail-open) for a caller
  that needs it mid-run, and `PauseKey(suffix)` is the one place the `freehire:pause:` prefix is
  spelled.

### Holding outbound mail by class

Mail workers fall into three classes (`internal/engage/emailnotify/pause.go`, the table is
`workerClass`), so an operator can keep payment confirmations flowing while digests and nudges
wait:

| Class | Key | Workers |
|---|---|---|
| transactional | `freehire:pause:mail:transactional` | `pro-welcome-mail` |
| engagement | `freehire:pause:mail:engagement` | `notify`, `message-notify`, `mentorship-remind` |
| lifecycle | `freehire:pause:mail:lifecycle` | `onboarding`, `nudge`, `remind`, `limit-nudge-mail`, `cv-lifecycle-mail` |
| all mail | `freehire:pause:mail:all` | every worker above |

```bash
redis-cli SET freehire:pause:mail:lifecycle 1 EX 86400   # hold digests-and-nudges class
redis-cli SET freehire:pause:mail:all 1                  # hold all outbound mail workers
redis-cli DEL freehire:pause:mail:lifecycle              # resume one class
```

`freehire:pause:<binary>` and `freehire:pause:all` keep working unchanged.

- **Whole-run holds**: `pro-welcome-mail`, `onboarding`, `limit-nudge-mail`, `cv-lifecycle-mail`, `mentorship-remind`
  and `message-notify` pass `emailnotify.HoldKeys(<binary>)` to `Main`, so a class hold takes the
  per-binary path: exit 0, `freehire_worker_paused=1`, no `last_run_*` series.
  `message-notify` is whole-run because one channel succeeding marks a notice as sent;
  withholding only email would lose it.
- **Email-only holds**: `notify`, `nudge` and `remind` also deliver over Telegram and push, so a
  class hold leaves the email channel out of the router (`emailnotify.EmailHeld`) and the other
  channels run. Email items soft-skip: they stay pending and no attempt counts toward
  dead-lettering. A `nudge`/`remind` batch that also succeeds on Telegram or push is delivered
  without its email copy, the same as for any email outage today.
- **Never held by a class**: the API server's mail (verification codes, password reset,
  application mail) and the hand-run `broadcast` / `mail-preview`. A guard test fails when a
  binary that builds an SES client is neither in the table nor exempt.
- **It fails open, as the per-binary key does.** While Redis is unreadable a held class sends.
  `FREEHIRE_IGNORE_PAUSE=1` lifts class holds too.
- **Resume order after an outage: transactional, then engagement, then lifecycle.** Delete the
  keys one class at a time and let a run complete before the next.
- `freehire_worker_paused` is unchanged; the `freehire-ops` panel is maintained separately.

### The SES circuit breaker

Workers build their SES client with `WithSendingBreaker()` (the API server does not: a latch in
a long-lived process would outlive the outage). The first `SendingPausedException`,
`AccountSuspendedException`, or `MessageRejected` saying sending is paused trips it, and every
later `Send` in the run returns `emailnotify.ErrSendingPaused` without calling SES. Any other
`MessageRejected` (bad address, unverified identity) is not account-level and does not trip it.
`onboarding` and `cv-lifecycle-mail` end their pass on it and write no ledger row (the latter also gives back its step row), so no slot is burned; per-address
failures still burn theirs. `notify`, `nudge` and `remind` map it to their channel soft-skip, so
no attempt is counted. `message-notify`, `pro-welcome-mail`, `limit-nudge-mail` and
`mentorship-remind` already leave the item retryable on any failed send. The breaker never
writes a hold key; setting one stays the operator's decision.
