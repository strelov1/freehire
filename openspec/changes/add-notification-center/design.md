## Context

Three notification engines (`internal/notify`, `internal/reminder`,
`internal/nudge`) each mark something delivered at one call site
(`notify.deliverOne`, `reminder.fire`, `nudge.fire`) after a successful
`Notifier.Send`. None of that is durably readable by the user afterward — it's
either in someone's inbox/Telegram history or gone. This change adds a fourth,
parallel thing that happens at each of those same three call sites: recording a
row the user can browse in-app, independent of which channel(s) carried it.

`add-push-notification-channel` (merged) already wrote the short, human-readable
title/body copy for each notification kind (for push); this change reuses that
copy verbatim rather than inventing a second wording for the same event.

## Goals / Non-Goals

**Goals:**
- One durable, readable-by-the-owner record per delivery event, regardless of
  channel.
- Read/unread state with a badge count, on both web and mobile.
- Reuse this codebase's existing pagination and mark-read idioms exactly
  (found in `internal/handler/handler.go`'s `pageParamsBounded`/`listResponse`
  and the Gmail inbox's `GetEmail`/`MarkAllReadInbox`), not invent new ones.

**Non-Goals:**
- Deduplicating `internal/notify`'s per-(saved_search, channel) subscription
  model. A user subscribed to the same saved search on both Telegram and push
  gets two `deliverOne` calls for the same matched jobs today, and will get two
  `user_notifications` rows for what feels like one event. Fixing this means
  either collapsing subscriptions to one row with multiple channels (a real
  schema change to `filter-subscriptions`, out of scope) or deduplicating at
  write time by some content hash (adds a real amount of complexity for a
  cosmetic near-duplicate). Accepted as-is.
- A dedicated unread-count-only endpoint. The list endpoint's `meta.unread_count`
  is cheap (one extra `COUNT(*) WHERE read_at IS NULL`) and both clients can
  call the list endpoint with a small `limit` to refresh the badge without a
  second endpoint to maintain.
- Push notifications themselves, retry/backoff, or anything in
  `internal/pushnotify` — this is a read-side feature over data that already
  exists; no delivery behavior changes.
- A tracking-board screen in `freehire-mobile` (none exists yet). See Decision
  5 for how mobile handles the two nudge kinds that would otherwise link there
  on web.

## Decisions

### 1. One `user_notifications` table, minimal columns

```
id, user_id, kind (text), title, body, public_slug (nullable), created_at, read_at (nullable)
```

No `job_id`. Every other place in this codebase that hands a job reference
back to a client (`DigestJob`) already carries only the public slug, not the
internal id — this table is read by its owner over an API, same rule applies,
and nothing here needs an internal id for a join (the row is self-contained:
title/body already have the job's title/company baked in as text, same as the
push copy they're copied from). `kind` is plain `text`
(`subscription_digest`/`reminder`/`nudge_follow_up`/`nudge_interview_prep`/
`nudge_job_closed`), matching how `application_nudges.kind` is already plain
text rather than a Postgres enum type elsewhere in this schema.

**Alternative considered:** a JSONB `payload` column for kind-specific extra
data. Rejected — every kind's rendering need is fully covered by
title/body/public_slug already; a JSONB blob is a seam for a need that does not
exist yet, and the AGENTS.md guidance is against building ahead of a concrete
need.

### 2. Recording happens via each engine's existing `Store` interface, not a new package

Each of `notify.Store`, `reminder.Store`, `nudge.Store` already lists the
handful of `*db.Queries` methods that engine calls (e.g.
`notify.Store.MarkMatchesNotified`). Add one more method,
`RecordNotification(ctx, arg) error`, to all three interfaces (all satisfied by
`*db.Queries` automatically, like the rest), and call it right next to the
existing `MarkMatchesNotified`/`MarkReminderDelivered`/`MarkNudgeDelivered`
call in each engine's delivery success path.

**Alternative considered:** a shared `internal/notifhistory` package with a
`Recorder` type, imported by all three engines (mirroring how
`internal/pushnotify` is shared transport code). Rejected — `pushnotify` earns
its own package because it does real work (HTTP calls, ticket tracking,
pruning); recording a notification is one `INSERT`, already behind each
engine's own `Store` seam. A package for a single generated method is the kind
of premature abstraction the "no overengineering" guidance calls out; if a
fourth caller shows up needing more than an insert, promoting it then costs
nothing lost.

**A recording failure must not fail the delivery it's recording.** The digest/
reminder/nudge itself was already sent — losing the in-app record of it is a
degraded read-side feature, not a reason to dead-letter a real delivery. Each
call site logs and continues, the same posture `deliverOne` already takes
toward `MarkMatchesNotified`'s own failure ("Delivered but not stamped ... the
lease expiry will re-deliver").

### 3. Pagination: offset/limit via the existing shared helpers

`GET /api/v1/me/notifications` uses `pageParamsBounded(c, defaultLimit,
maxLimit)` and a `listResponse`-shaped envelope extended with `unread_count` —
the same pattern `GetInbox` already uses (`listResponseWithHidden` is the
precedent for "one more meta field, still one query", not a generic list
endpoint fetching a second count via a second query call is avoidable here
since the count is a `COUNT(*) FILTER (WHERE read_at IS NULL)` addable to the
same query the page already runs, same shape as `GetInbox`'s `Total`/`Hidden`
already being one query away).

**Alternative considered:** cursor-based, matching `internal/handler/community.go`.
Rejected — that pattern is reserved for a public, unauthenticated high-volume
feed in this codebase; every owner-scoped `/me/*` list here (inbox included)
uses offset/limit, and there's no reason for notifications to be the
exception.

### 4. Read/unread: dedicated POST sub-routes, mirroring the inbox exactly

- `POST /api/v1/me/notifications/:id/read` — idempotent
  (`UPDATE user_notifications SET read_at = now() WHERE id = $1 AND user_id =
  $2 AND read_at IS NULL`), same shape as `MarkEmailRead`.
- `POST /api/v1/me/notifications/read-all` — same shape as
  `MarkAllReadInbox`, returns `{"data": {"marked": n}}`.

Opening the list does NOT mark anything read (unlike `GetEmail`, which marks
on open) — a notification card's read state is a deliberate user action (tap
the card, or tap "mark all read"), not an implicit side effect of fetching a
page, since a client polling the list for the badge count must not silently
zero it.

### 5. Deep-link target per kind

A card's tap target follows the job reference it carries:

- `subscription_digest` with a single matched job (per the push change's
  existing `Total == 1` rule) → that job. With more than one matched job, no
  `public_slug` is stored and the card has nothing to open (matches the push
  change's precedent of no deep-link for a multi-job digest).
- `reminder` → always has a slug (a reminder always concerns one job) → that
  job.
- `nudge_job_closed` → the job (nothing left to track once closed, same as
  the existing Telegram/email copy's own link choice).
- `nudge_follow_up` / `nudge_interview_prep` → **web**: `/my/tracking` (the
  tracking board), matching the existing Telegram/email link target exactly —
  the row still stores the job's slug (a `nudge.Message` always carries one),
  it's just not what the web UI links to for these two kinds. **Mobile**: no
  tracking-board screen exists yet in `freehire-mobile`, so these two kinds
  link to the job page instead, same as `nudge_job_closed` — a real job page
  is a strictly better target than nothing, even if a future tracking screen
  would be better still. This is a per-platform card-rendering choice, not a
  stored-data difference (`public_slug` is populated the same way for all
  three nudge kinds either way).

### 6. Migration number

Next is `0090` — confirmed against `origin/main`'s `migrations/` tree (not the
local `migrations/` listing, which can lag or duplicate across in-flight
branches per this repo's own migration-numbering gotcha).

## Risks / Trade-offs

- **[Risk] The known `internal/notify` per-channel duplicate (Non-Goals) reads
  as a bug to a user who enabled two channels on one saved search.** →
  Mitigation: acceptable for v1; the same duplication already exists in
  spirit (they get two real messages today, over Telegram and push, for the
  same match) — the notification center just makes that visible in one place
  instead of two separate apps.
- **[Risk] A recording INSERT on every delivery adds write volume to a new
  table on the hot delivery path of three cron workers.** → Mitigation: one
  row, one indexed insert, no different in shape or cost from the
  `subscription_matches`/`job_reminders`/`application_nudges` writes already
  happening on the same path; not a new order of magnitude.
- **[Trade-off] No JSONB payload column (Decision 1) means a future kind
  needing structured extra data requires a migration.** → Accepted: the
  current five kinds all fit title/body/slug; adding a column later is cheap
  and this schema has no `NOT NULL` constraints that would make one hard to
  add.

## Migration Plan

One migration (`0090_user_notifications.sql`): `CREATE TABLE user_notifications`
with an index on `(user_id, created_at DESC)` for the list query and
`(user_id) WHERE read_at IS NULL` (partial index) for the unread count. No
backfill — the table starts empty; existing deliveries before this ships are
not retroactively recorded, so a user's notification center starts at zero on
first deploy, same as every list-based feature in this codebase that shipped
after data already existed.

## Open Questions

None outstanding — scope, architecture, and platform-specific UI decisions
were confirmed with the user, and the remaining implementation-detail calls
(pagination style, mark-read shape, migration number, nudge-kind mobile
target) are resolved above by matching this codebase's existing conventions.
