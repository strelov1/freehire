## 1. Reminder scheduling — bucket the fire time

- [x] 1.1 Add a `GetReminderScheduleContext` query to `internal/platform/db/queries/reminders.sql` returning the account's `notification_settings.digest_time` and `users.timezone` in one read; run `make sqlc`
- [x] 1.2 Extend `reminder.Repository` and `QueriesRepository` with a `GetScheduleContext(ctx, userID)` returning the notification hour and IANA timezone, defaulting to 09:00 and UTC when either is unset
- [x] 1.3 Round `Service.fireAt` forward to the first notification hour at or after `now + DefaultDelayDays` in the account's timezone, and wire `ScheduleOnSave` to it
- [x] 1.4 Cover the rounding with unit tests: default 09:00/UTC, a configured `digest_time` in a non-UTC zone, two saves hours apart on one day landing on the same fire time, and rounding never moving the fire time earlier than the delay

## 2. Reminder delivery — group by user

- [x] 2.1 Change `reminder.Notifier`/`Router` to take `[]ReminderMessage`, and update the compile-time assertions
- [x] 2.2 Add `SnapshotCap` (default 200) to `reminder.Config`; reuse `notify.ListLimit` for the message list bound
- [x] 2.3 Split `Runner.fire` into a per-item validation step (load, `job_open`/`still_actionable`, quiet hours) and a per-group send, and make `Runner.Run` group survivors by `user_id` oldest-due first
- [x] 2.4 Record one in-app notification per group: `jobs` set and `public_slug` unset for a multi-item group, today's shape for a single-item group
- [x] 2.5 Mark every item in a delivered group delivered, and record a delivery failure against every item in a failed group
- [x] 2.6 Cover with tests: two jobs of one user is one `Send`, two users is two `Send`s, a cancelled item leaves the group while the rest still deliver, a failed group attempts every item, a group of one keeps the single-job record shape

## 3. Reminder transports — render a list

- [x] 3.1 Render the email body as a list of `mailtpl` job rows, itemizing at most `notify.ListLimit` with an "and N more" tail, and adjust the subject and preheader for a multi-job group
- [x] 3.2 Render the Telegram message as a list under the existing UTF-16 length cap, reserving the widest possible tail up front
- [x] 3.3 Render the push notification as a single summary carrying the group's count
- [x] 3.4 Cover each transport with tests for a group of one, a group under the list limit, and a group over it

## 4. Nudge delivery — group by (user, kind)

- [x] 4.1 Change `nudge.Notifier`/`Router` to take a kind plus `[]Message`, and update the compile-time assertions
- [x] 4.2 Add `SnapshotCap` (default 200) to `nudge.Config`
- [x] 4.3 Split `Runner.fire` the same way as the reminder engine and group survivors by `(user_id, kind)` oldest-due first in `Runner.deliver`
- [x] 4.4 Record one in-app notification per group, keeping the existing `nudge_<kind>` kind string
- [x] 4.5 Cover with tests: four same-kind nudges is one `Send`, two kinds is two `Send`s, and each kind's record keeps its own `nudge_<kind>` value

## 5. Nudge transports — render a list

- [x] 5.1 Render email, Telegram and push for a group, per kind, with the same list bound and tail as the reminder transports
- [x] 5.2 Cover each transport and kind with a group-of-one and a group-over-the-limit test

## 6. Wire-up and documentation

- [x] 6.1 Update `cmd/remind` and `cmd/nudge` for the new notifier signatures and config fields
- [x] 6.2 Record the batching rule in `docs/agents/notifications.md`: what forms a group, why nudge kinds stay apart, the two bounds, and that the group is the retry unit
- [x] 6.3 Run `gofmt -l .`, `go vet ./...`, `go test ./...` and `go vet -tags=integration ./...`; run the integration-tagged tests for `internal/engage/...`
