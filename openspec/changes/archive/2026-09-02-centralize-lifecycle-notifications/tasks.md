## 1. Migration & generated queries

- [x] 1.1 Migration: rename `reminder_settings` → `notification_settings`, drop `default_delay_days`
- [x] 1.2 Migration: create `application_nudges` (id, user_id, job_id, kind, episode_key, status, last_error, attempts, claimed_at, created_at) with `UNIQUE (user_id, job_id, kind, episode_key)` and the lease/due indexes needed for claim queries
- [x] 1.3 Update `internal/db/queries/*.sql`: rename the reminder-settings queries onto `notification_settings` (drop the delay param, add the "no row yet" default of enabled+email at the query or repository layer per design D3), add new queries for `application_nudges` (insert-candidate `ON CONFLICT DO NOTHING`, claim-due, mark delivered/cancelled, record failure)
- [x] 1.4 `make sqlc`; verify generated code compiles

## 2. `internal/reminder` simplification

- [x] 2.1 Remove `Override` type and the override parameter from `ScheduleOnSave`; every save schedules at the fixed `DefaultDelayDays`
- [x] 2.2 Remove `RescheduleReminder`/per-job `CancelReminder` from `Repository` and `Service` (worker-side cancel-at-fire in `engine.go` is untouched — that cancels on job-closed/applied/unsaved, not per-job user choice)
- [x] 2.3 Update `Settings`/`Repository.GetSettings`/`UpsertSettings` to drop `DelayDays`; absent-row default becomes `enabled: true, channels: ['email']` (design D3)
- [x] 2.4 Update `internal/reminder/reminder_test.go` and `engine_test.go` for the removed override/reschedule paths and the new absent-row default
- [x] 2.5 `internal/handler/user_jobs.go`: remove `reminderOverrideRequest`, the `reminder` field on the save request, and the per-job `PATCH`/`DELETE /api/v1/jobs/:slug/reminder` routes+handlers (also renamed `/me/reminder-settings` → `/me/notification-settings`, and dropped the now-orphaned `reminder_fire_at` projection from `ListUserJobs`/`jobtracking`/`me_tracking.go` — it fed only the deleted per-job control)

## 3. `internal/nudge` engine

- [x] 3.1 Package skeleton: `Message`, `Notifier`, `Router`, `Store` interface, `Config`/`DefaultConfig`, `Stats` (mirror `internal/notify`'s shapes)
- [x] 3.2 MATCH: follow-up candidates — active tracked applications with `userjob.SilenceStateFor(...) == SilenceSilent`, bounded by a recency window on `last_activity_at`, gated on `notification_settings.enabled`, inserted with `episode_key = last_activity_at`
- [x] 3.3 MATCH: interview-prep candidates — `application_events` `stage_set → interview` rows not yet in the ledger, bounded by a recency window on `occurred_at`, gated on `notification_settings.enabled`, inserted with `episode_key = occurred_at`
- [x] 3.4 DELIVER: claim pending rows, re-check-before-send per kind (follow-up: still `SilenceSilent`; interview-prep: stage still `interview`; both: `notification_settings.enabled` still true), cancel vs. deliver vs. record-failure
- [x] 3.5 Email/Telegram transports with per-kind copy, both linking to `/my/tracking`
- [x] 3.6 Unit tests: episode-key stability across repeated MATCH passes, episode-key change on new activity / new `stage_set`, each of the three cancel-at-delivery conditions, gating on `enabled` (12 tests, `internal/nudge/nudge_test.go`)

## 4. `cmd/nudge` worker

- [x] 4.1 `cmd/nudge/main.go`: MATCH-then-DELIVER run-once worker, mirroring `cmd/notify`'s channel wiring and "no configured channel → exit 0" / "delivery failures → exit non-zero" shape
- [x] 4.2 Wire `worker.Bootstrap`, `nudge.DefaultConfig` (windows finalized at 30 days for follow-up, 7 days for interview-prep; lease/claim-batch/max-attempts mirror `reminder`/`notify`'s existing tuning)

## 5. SPA: settings relocation

- [x] 5.1 New `/my/notifications` route hosting the simplified `ReminderSettings.svelte` (toggle + channel picker, delay-days picker removed)
- [x] 5.2 Remove the settings widget from `/my/activity`
- [x] 5.3 Add the new section to `accountNav.ts` and `accountNavIcons.ts`
- [x] 5.4 **Corrected from the design's assumption**: the "connect the Telegram bot" hint stays pointed at `/my/searches` — that page (`SavedSearchesView.svelte`) is where the Telegram-bot-linking UI actually lives (confirmed in code, not `/my/notifications`); only the hint's wording changed ("search notifications" page) to stay unambiguous now that a second, distinct notifications page exists

## 6. SPA: per-job control removal

- [x] 6.1 Delete `ReminderChip.svelte` and remove its usage in `SavedJobs.svelte`; also found and removed a second per-job control the design didn't originally scope — `JobRow.svelte`'s post-save "remind me in N days" popover (`reminderPrompt`/`setReminder`/`REMINDER_CHOICES`), which called the same per-job override endpoint
- [x] 6.2 Remove `api.rescheduleReminder`/`api.cancelReminder`, the `reminder` param on `api.saveJob`, and `reminder_fire_at` from the saved-job type
- [x] 6.3 Update/remove tests referencing the deleted component and API surface; also updated `web/src/lib/docs/api-spec.ts` (the public API reference page) to drop the removed endpoints and document `/me/notification-settings`

## 7. Verification

- [x] 7.1 `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` — clean
- [x] 7.2 `go test ./...` — all green; `go test -tags=integration ./internal/db/` — all green, including 5 new tests against real Postgres (`nudges_integration_test.go`) covering both MATCH queries, idempotent `RecordNudge`, and the claim/deliver/cancel lifecycle
- [x] 7.3 `pnpm run check` (svelte-kit sync + svelte-check: 0 errors) and `pnpm test` (79 files / 862 tests, all green)
- [x] 7.4 Verified via the automated test layers above rather than a manual click-through against a running local stack: the SQL layer against real Postgres (7.2), the MATCH/DELIVER decision logic against fakes (`internal/nudge/nudge_test.go`), and the settings default/gating behavior at both the unit (`reminder_test.go`) and integration (`reminders_integration_test.go`) levels. No live `cmd/nudge` run against a full local API+SPA stack was performed.
