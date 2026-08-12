## 1. Database layer

- [x] 1.1 Migration `0090_user_notifications.sql`: `CREATE TABLE user_notifications (id bigint identity PK, user_id bigint NOT NULL, kind text NOT NULL, title text NOT NULL, body text NOT NULL, public_slug text, created_at timestamptz NOT NULL DEFAULT now(), read_at timestamptz)`, index on `(user_id, created_at DESC)`, partial index on `(user_id) WHERE read_at IS NULL`
- [x] 1.2 sqlc queries in `internal/db/queries/notifications.sql`: `RecordNotification :exec` (insert), `ListUserNotifications :many` (offset/limit, newest first, with a window-function or companion query for `total`), `CountUnreadNotifications :one`, `MarkNotificationRead :execrows` (owner-scoped, idempotent `WHERE read_at IS NULL`), `MarkAllNotificationsRead :execrows` (owner-scoped, unread-only); regenerate (`make sqlc`)

## 2. Engine wiring (write side)

- [x] 2.1 Add `RecordNotification(ctx, arg db.RecordNotificationParams) error` to `notify.Store`; call it in `deliverOne` right after a successful `MarkMatchesNotified`, building `kind=subscription_digest`, the already-rendered title/body (extract the notify push-copy render logic into a shared, channel-agnostic helper if that's cleaner than duplicating the `fmt.Sprintf` — see push.go), and `public_slug` only when the digest matched exactly one job; log-and-continue on error, never fail the delivery; unit test with a fake `Store` asserting the call happens with the right kind/slug for 1-job vs N-job digests
- [x] 2.2 Same for `internal/reminder`: `RecordNotification` on `reminder.Store`, called from `fire`'s delivered branch, `kind=reminder`, slug always set; unit test
- [x] 2.3 Same for `internal/nudge`: `RecordNotification` on `nudge.Store`, called from `fire`'s delivered branch, `kind` mapped from `Message.Kind` (`nudge_follow_up`/`nudge_interview_prep`/`nudge_job_closed`), slug always set; unit test covering all three kinds
- [x] 2.4 Integration test (one file, exercising all three engines against a real Postgres via `testdb.Pool`): a delivered digest/reminder/nudge each produces exactly one `user_notifications` row with the expected kind/slug; a forced recording failure (e.g. a bad FK) does not prevent the delivery from being marked notified

## 3. Backend read/write API

- [x] 3.1 `internal/handler/me_notifications.go`: `GetNotifications` (`GET /me/notifications`, `pageParamsBounded` + `listResponse`-style envelope with `meta.unread_count`), `MarkNotificationRead` (`POST /me/notifications/:id/read`, owner-scoped, 404 for another user's, idempotent), `MarkAllNotificationsRead` (`POST /me/notifications/read-all`, returns `{"data":{"marked":n}}`) — all cookie-only (`mw.cookie`, matching the rest of `/me/*`)
- [x] 3.2 Register the three routes; unit tests for each handler (fake store) covering: default page, pagination bounds, owner-scoping 404, idempotent mark-read, mark-all-read count
- [x] 3.3 Integration test (`//go:build integration`): seed rows across users, confirm list/pagination/unread_count/mark-read/mark-all-read against real Postgres and real owner-scoping

## 4. Web UI

- [x] 4.1 API client functions in `web/src/lib/api.ts`: `getNotifications(limit?, offset?)`, `markNotificationRead(id)`, `markAllNotificationsRead()`
- [x] 4.2 A `notifications.svelte.ts` store (or extend an existing one) holding the unread count + page, following this codebase's existing `.svelte.ts` store conventions (see `notifications.svelte.ts` already used by `AlertChannels.svelte` for the unrelated subscription store — pick a non-colliding name/location)
- [x] 4.3 Bell icon + badge component in the header chrome; a list view/panel rendering notification cards (kind icon, title/body, relative time, unread state), wired to mark-read on tap and the per-kind navigation target from design.md decision 5 (job page, or `/my/tracking` for the two web-only nudge kinds)

## 5. Mobile UI (`freehire-mobile`, separate repo)

- [x] 5.1 API client functions in `src/lib/api.ts`: `getNotifications`, `markNotificationRead`, `markAllNotificationsRead`, matching the existing `send`/`requestData` helpers' style
- [x] 5.2 Bell icon with badge in `src/app/index.tsx`'s header row, alongside the existing account icon (same `Pressable`/`SymbolView` pattern, badge reusing the existing filter-badge styles)
- [x] 5.3 New screen `src/app/notifications.tsx` (or similar route) listing notification cards; register it in `_layout.tsx`'s `Stack` as a modal/pushed screen like `account`/`filters`
- [x] 5.4 Tap-to-navigate: job-bearing cards → `/jobs/[slug]` (including `nudge_follow_up`/`nudge_interview_prep`, per design.md decision 5's mobile fallback); no-slug cards → no navigation, just mark read

## 6. Verification

- [x] 6.1 `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, `go vet -tags=integration ./...` clean
- [x] 6.2 `go test -tags=integration ./...` (full module) clean
- [x] 6.3 Web `eslint`/`svelte-check` clean on changed/new files
- [x] 6.4 Mobile `jest`/`tsc`/`eslint` clean on changed/new files
- [ ] 6.5 Manual smoke on a real device + web: trigger a delivery (as done for the push-channel change), confirm it appears in both notification centers, confirm badge count, mark-read, and tap-to-navigate for a job-bearing and a tracking-board-bound (web) card
