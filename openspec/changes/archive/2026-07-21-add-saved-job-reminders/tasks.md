## 1. Data model & queries

- [x] 1.1 Add migration for `reminder_settings` (user_id PK, enabled, default_delay_days, channels text[], updated_at)
- [x] 1.2 Add migration for `job_reminders` (id, user_id, job_id, fire_at, channels text[], status, claimed_at, attempts, failed_at, last_error, created_at, delivered_at) with a partial unique index on `(user_id, job_id) WHERE status='pending'`
- [x] 1.3 Write sqlc queries: get/upsert reminder settings
- [x] 1.4 Write sqlc queries: upsert pending reminder, reschedule, cancel by (user,job), get delivery context with fire-time re-check flags
- [x] 1.5 Write sqlc queries for the firing engine: claim due pending reminders (`fire_at<=now()`, lease via FOR UPDATE SKIP LOCKED reclaiming dead leases), mark delivered, record delivery failure/dead-letter
- [x] 1.6 Regenerate sqlc (`make sqlc`) and confirm build

## 2. Reminder settings (account default rule)

- [x] 2.1 `internal/reminder` settings service: read rule, update rule (validate channels against `notify.Channels`, delay bounds)
- [x] 2.2 Handlers `GET/PUT /me/reminder-settings` (cookie auth) + route wiring
- [x] 2.3 Handler tests for read/update, including the off-by-default (absent row) case

## 3. Scheduling & cancellation (reminder.Service, handler-orchestrated)

- [x] 3.1 `reminder.Service.ScheduleOnSave(userID, jobID, override)`: read rule, resolve effective fire time (override delay | account default | opt-out), upsert or cancel the pending reminder
- [x] 3.2 `reminder.Service.Cancel(userID, jobID)` for the eager apply/unsave path (idempotent)
- [x] 3.3 Orchestrate in HTTP handlers: save → `ScheduleOnSave` (override from request body); apply & unsave → `Cancel`
- [x] 3.4 Handle job-closure cancellation lazily at fire time (worker re-check), not via hooks in the scattered close paths — see design decision 2
- [x] 3.5 Service tests: default-delay schedule, override delay, opt-out cancels, re-save replaces, off-by-default schedules nothing
- [x] 3.6 Handler tests: save schedules per override, apply/unsave cancel

## 4. Per-job reminder management

- [x] 4.1 Handlers `PATCH /jobs/:slug/reminder` (reschedule) and `DELETE /jobs/:slug/reminder` (cancel) + routes
- [x] 4.2 Project each saved row's pending reminder (`fire_at` or none) into the `filter=saved` tracking listing response
- [x] 4.3 Handler tests for reschedule/cancel and the listing projection (listing projection verified in the integration pass 8.2)

## 5. Firing engine & transports

- [x] 5.1 `internal/reminder` engine `Runner.Run`: claim due reminders → deliver → mark delivered; retry/dead-letter on failure; re-check saved-not-applied-and-open just before send
- [x] 5.2 `ReminderMessage` type + reminder `Notifier` seam and `Router` (channel → impl)
- [x] 5.3 Telegram reminder notifier reusing `telegramnotify.Client.SendMessage`; email reminder notifier reusing the `emailnotify` SES sender; resolve destinations live (soft-skip when absent)
- [x] 5.4 Engine tests with a fake Notifier/Store: one-shot delivery, idempotent re-run, soft-skip on missing destination, not-yet-due, cancel-race skip

## 6. Worker

- [x] 6.1 `cmd/remind/main.go` mirroring `cmd/notify` (`worker.Main`/`Bootstrap`/`ExitCode`), no-op-and-exit-0 when unconfigured
- [x] 6.2 Manual run against a seeded reminder confirms delivery + ledger marking

## 7. Frontend

- [x] 7.1 Non-modal save affordance with quick choices (default / tomorrow / +week / don't remind) on job save
- [x] 7.2 Reminder chip + reschedule/off controls on saved job rows (`SavedJobs.svelte` / `JobRow`)
- [x] 7.3 Reminder settings block on `/my/activity` (enable, default delay, channels) wired to `/me/reminder-settings`
- [x] 7.4 API client methods for settings + per-job reminder endpoints

## 8. Verification & rollout

- [x] 8.1 `go build ./... && go vet ./... && go test ./...` green
- [x] 8.2 End-to-end check: save with each override, confirm scheduled row; trigger apply/unsave/close and confirm cancellation; run `cmd/remind` and confirm one-shot delivery
- [x] 8.3 Note prod scheduling for `cmd/remind` (cron/systemd timer, ~15 min) and the manual migration step
