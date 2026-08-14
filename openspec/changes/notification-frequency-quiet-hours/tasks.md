## 1. Migration & generated queries

- [x] 1.1 Migration: `users.timezone text` (nullable); `notification_settings` gains `digest_frequency text NOT NULL DEFAULT 'instant'`, `digest_time time`, `quiet_hours_start time`, `quiet_hours_end time`; `subscriptions` gains `last_digest_sent_at timestamptz` (nullable). All additive, no backfill.
- [x] 1.2 `internal/db/queries/users.sql`: add `UpdateUserTimezone :one` (validated IANA name comes in pre-validated from the handler; the query just stores the text).
- [x] 1.3 `internal/db/queries/reminders.sql`: extend `UpsertNotificationSettings` to also accept/store `digest_frequency`, `digest_time`, `quiet_hours_start`, `quiet_hours_end` in the same upsert as `enabled`/`channels` (one account-level settings row, one write path).
- [x] 1.4 `internal/db/queries/subscriptions.sql`: extend `GetSubscriptionForDelivery` with a `LEFT JOIN notification_settings ns ON ns.user_id = s.user_id`, selecting `ns.digest_frequency`, `ns.digest_time`, `ns.quiet_hours_start`, `ns.quiet_hours_end`, `u.timezone`, and `s.last_digest_sent_at`; add `MarkDigestSent :exec` (or fold into `MarkMatchesNotified`) to stamp `last_digest_sent_at = now()`.
- [x] 1.5 `internal/db/queries/reminders.sql`: extend `GetReminderForDelivery` with the same `LEFT JOIN notification_settings` + `u.timezone`, selecting `quiet_hours_start`/`quiet_hours_end` (no digest fields — reminders don't read frequency).
- [x] 1.6 `internal/db/queries/nudges.sql`: extend the existing `GetNudgeForDelivery` (already joins `notification_settings`) with `ns.quiet_hours_start`, `ns.quiet_hours_end`, `u.timezone`.
- [x] 1.7 `make sqlc`; verify generated code compiles.
- [x] 1.8 `internal/db/queries/users.sql`: `CreateUser` gains an optional `timezone` param (nullable) — the browser-detected zone captured at signup (see task 6.5), stored at account creation instead of left NULL until a profile-page visit.

## 2. `internal/deliverywindow` (new leaf package)

- [x] 2.1 New package `internal/deliverywindow`: `InQuietHours(now time.Time, tz string, start, end *time.Time) bool` — nil `start`/`end` means quiet hours off (always false); handles overnight wraparound (`start > end`); nil/invalid `tz` falls back to UTC.
- [x] 2.2 Same package: `DigestDue(now time.Time, tz string, digestTime *time.Time, lastSentAt *time.Time) bool` — true when local time has passed `digestTime` and `lastSentAt` is nil or its local calendar date precedes today's local calendar date.
- [x] 2.3 Unit tests: quiet-hours same-day window, overnight-wraparound window (both sides of midnight), quiet-hours-off, invalid/empty timezone falls back to UTC, digest-due before/after the time boundary, digest-due same-day-already-sent vs next-day, digest-due with nil `lastSentAt`.

## 3. `internal/notify` wiring

- [x] 3.1 `deliverOne`: after loading `GetSubscriptionForDelivery`, if `digest_frequency == "instant"` and `deliverywindow.InQuietHours(...)`, release the claim and soft-skip (do not send, do not fail); if `digest_frequency == "daily"` and `!deliverywindow.DigestDue(...)`, release and soft-skip (quiet hours does NOT gate `daily`, per design).
- [x] 3.2 On a successful `daily`-mode delivery, stamp `last_digest_sent_at` (the task 1.4 query) alongside the existing `MarkMatchesNotified` call.
- [x] 3.3 Distinguish this soft-skip reason from the existing "channel not configured" one in `Stats` (e.g. a `Deferred` counter) so `cmd/notify`'s run summary can tell them apart.
- [x] 3.4 Unit tests (fake `Store`): instant + quiet hours defers; instant + no quiet hours delivers; daily + before time defers; daily + due delivers and stamps `last_digest_sent_at`; daily + already sent today defers; daily ignores quiet hours.

## 4. `internal/reminder` wiring

- [x] 4.1 `fire`: after loading `GetReminderForDelivery` and the existing `JobOpen`/`StillActionable` re-check, if `deliverywindow.InQuietHours(...)`, release the claim (same `r.release` already used for the "no usable destination" soft-skip) rather than deliver.
- [x] 4.2 Unit tests: due reminder deferred during quiet hours, delivered outside quiet hours, quiet-hours-off unaffected.

## 5. `internal/nudge` wiring

- [x] 5.1 `fire`: after the existing `actionable(info)` re-check (which stays a cancel, not a defer), add the same quiet-hours check as reminder's — release, don't cancel, don't deliver.
- [x] 5.2 Unit tests: due nudge deferred during quiet hours (covering at least one of the three kinds), delivered outside quiet hours.

## 6. Backend API

- [x] 6.1 `internal/handler/me_reminders.go` (or wherever `/me/notification-settings` lives): extend the request/response shape for `digest_frequency`/`digest_time`/`quiet_hours_start`/`quiet_hours_end`; validate `digest_frequency ∈ {instant, daily}`, `digest_time` required when `daily`, `quiet_hours_start`/`quiet_hours_end` set together or not at all.
- [x] 6.2 New `PATCH /me/timezone` (cookie-only, matching this codebase's narrow-single-purpose `/me/*` PATCH endpoints): validates the IANA name via `time.LoadLocation`, 400 on invalid, updates `users.timezone`.
- [x] 6.3 Unit tests for both handlers: valid/invalid frequency+time combinations, valid/invalid timezone strings, owner-scoping (implicit via cookie auth).
- [x] 6.4 Integration test (`//go:build integration`): round-trip both endpoints against real Postgres.
- [x] 6.5 `credentials`/`accounts.Register` gain an optional `timezone` param (an invalid/empty value is silently ignored, never a 400 — a browser quirk must not block signup); the web registration flow sends `Intl.DateTimeFormat().resolvedOptions().timeZone` alongside email/password. OAuth sign-up (`ResolveOAuthAccount`) is out of scope for this change (no existing web-side hook to capture it from) — those accounts get a timezone the first time they visit `/my/profile`, same as any pre-existing account.

## 7. Web: profile timezone field

- [x] 7.1 API client: `api.updateTimezone(tz: string)`.
- [x] 7.2 `ProfileForm.svelte`: a timezone field (searchable select over the IANA list) on the Settings tab, autosaving like the rest of the form's fields. When the account has no stored timezone yet, the field pre-fills with the browser-detected zone (`Intl.DateTimeFormat().resolvedOptions().timeZone`) as its shown/selected value rather than blank — so a plain "Save" on the rest of the form persists the detected zone even if the user never deliberately touches this field.

## 8. Web: notification settings frequency + quiet hours UI

- [x] 8.1 API client: extend `getNotificationSettings`/`updateNotificationSettings` types for the four new fields.
- [x] 8.2 `ReminderSettings.svelte` (or a new sibling block on `/my/notifications/settings`): frequency control (instant/daily) with a time picker revealed for daily; an optional quiet-hours start/end pair; a hint pointing at `/my/profile` when daily or quiet hours is enabled without a stored timezone.

## 9. Verification

- [x] 9.1 `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, `go vet -tags=integration ./...` clean.
- [x] 9.2 `go test -tags=integration ./...` (full module) clean.
- [x] 9.3 Web `eslint`/`svelte-check` clean on changed/new files.
- [x] 9.4 Manual smoke against a local backend+DB: set a timezone, switch to daily frequency with a near-future time, confirm a pending match waits and then delivers at that time; set quiet hours spanning the current time, confirm a reminder/nudge defers and delivers once the window ends.
