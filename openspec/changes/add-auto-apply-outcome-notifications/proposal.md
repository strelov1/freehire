## Why

`cmd/auto-apply` resolves a queued attempt unattended, on its own 5-minute cron. Today
the candidate learns the outcome — submitted for real, permanently blocked on a
required question, or dead-lettered after retries — only by opening `/my/tracking`
and reading the job's card or drawer; nothing pushes it to them. The job page's own
auto-apply "queued" banner already promises "you'll get a notification to review it,"
but no such notification exists yet for any of the three outcomes that end an
attempt, so the product does not keep that promise today.

## What Changes

- Extend the existing `internal/engage/nudge` lifecycle-notification engine
  (MATCH-scans-state, DELIVER-over-configured-channels, dedup by
  `(user, job, kind, episode_key)`) with three new nudge kinds:
  `auto_apply_submitted`, `auto_apply_blocked`, `auto_apply_failed`. No new
  notification engine, worker, or timer — these ride the existing
  `freehire-nudge.timer` (every 30 minutes).
- Add a new event source, `appevent.SourceAutoApply`, and have
  `cmd/auto-apply/store.go`'s successful-submission path (`Submit`) write it instead
  of the shared `appevent.SourceSystem`, so a submitted auto-apply is a distinct,
  queryable fact rather than an accidental side effect of `kind='applied'` always
  meaning auto-apply today.
- Add three MATCH queries: one reading `application_events` for the new source (the
  submitted case — the queue row is deleted on success, so the event is the only
  durable trace), two reading `auto_apply_queue.blocked_at`/`failed_at` directly
  (both permanently terminal once set, so no new "already notified" guard column is
  needed on that table).
- Widen `application_nudges.kind`'s CHECK constraint (migration) to accept the three
  new values.
- No frontend changes: the in-app notification bell already renders any nudge kind
  generically from server-rendered title/body, and already deep-links to the
  specific job when a batch names exactly one (always true here).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `lifecycle-nudges`: three new one-shot nudge kinds are added alongside the
  existing follow-up and interview-prep ones — matched from auto-apply's own
  terminal state (a successful submission, a permanently blocked attempt, or a
  dead-lettered failure) rather than from application silence or a stage
  transition, delivered under the same notification-setting gate and one-shot
  delivery guarantee the existing kinds already have.

## Impact

- `internal/application/appevent`: new `SourceAutoApply` constant.
- `cmd/auto-apply/store.go`: one call site's `EventSource` argument changes.
- `internal/engage/nudge`: `Store` interface, `Config`, `Runner.match`,
  `Runner.actionable`, and the email/Telegram/push rendering functions
  (`transports.go`, `push.go`) each gain three new cases, mirroring the existing
  `job_closed` kind's shape.
- `internal/platform/db/queries/nudges.sql` (+ generated `internal/platform/db`):
  three new queries.
- One new migration widening `application_nudges_kind_check`.
- No API surface change, no frontend change, no new deploy artifact.
