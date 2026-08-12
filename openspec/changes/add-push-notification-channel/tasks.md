## 1. Shared push transport (`internal/pushnotify`)

- [x] 1.1 Add a `data map[string]string` parameter to `pushnotify.Notifier.Send` and `ExpoNotifier.Send`; add `Data map[string]string \`json:"data,omitempty"\`` to `expoMessage`
- [x] 1.2 Update `TestPushToken` (`internal/handler/me_push_tokens.go`) to pass `nil` for the new parameter
- [x] 1.3 Add `pushnotify.SendToDevices(ctx, notifier Notifier, tokens []string, title, body string, data map[string]string) error` — sends to every token, returns nil if any succeeded, an aggregate error if all failed/were pruned; unit tests with a fake `Notifier` covering all-success, partial-success, all-pruned, all-failed, empty-tokens

## 2. Shared channel vocabulary

- [x] 2.1 Add `ChannelPush = "push"` to `internal/notify` and append it to `notify.Channels`; confirm `notify.ValidChannel("push")` is true and existing reminder/nudge validation (which already reuses this slice) accepts it

## 3. `internal/notify` push channel (filter-subscriptions)

- [x] 3.1 Add a `HasPushDevice bool` column to the `GetSubscriptionForDelivery` sqlc query (`EXISTS(SELECT 1 FROM user_push_tokens WHERE user_id = s.user_id)`), regenerate (`make sqlc`)
- [x] 3.2 Add a `ChannelPush` case to `notify.recipient()` — returns `(userID string, true)` when `HasPushDevice`, else `("", false)`; unit tests for both branches
- [x] 3.3 Implement `notify.PushNotifier`: renders title="freehire", body="{Total} new jobs for \"{SavedSearchName}\"", sets deep-link data to the sole job's slug when `Total == 1` and omits it otherwise, then calls `pushnotify.SendToDevices`; unit tests for the render logic (0/1/N-job digests) with a fake token store + fake `pushnotify.Notifier`
- [x] 3.4 Register `notify.PushNotifier` in `cmd/notify`'s `Router` unconditionally (no env-var gate, unlike Telegram/email)
- [x] 3.5 Integration test (`internal/db` + `internal/notify`, `//go:build integration`): a push subscription with a registered device is delivered; one with no device soft-skips

## 4. `internal/reminder` push channel (saved-job-reminders)

- [x] 4.1 Add a `HasPushDevice bool` column to the `GetReminderForDelivery` sqlc query, regenerate
- [x] 4.2 Add a `ChannelPush` case to `reminder.recipient()`, mirroring 3.2; unit tests
- [x] 4.3 Implement `reminder.PushNotifier`: short title/body ("⏰ Reminder" / "You saved {JobTitle} at {Company} — still interested?"), deep-link data always set to the reminder's job slug, calls `pushnotify.SendToDevices`; unit tests
- [x] 4.4 Register `reminder.PushNotifier` in `cmd/remind`'s `Router` unconditionally
- [x] 4.5 Integration test: a reminder rule with `push` in its channel set delivers to a registered device and soft-skips with none

## 5. `internal/nudge` push channel (mobile-push-channel: application nudge delivery)

- [x] 5.1 Add a `HasPushDevice bool` column to the `GetNudgeForDelivery` sqlc query, regenerate
- [x] 5.2 Add a `ChannelPush` case to `nudge.recipient()`, mirroring 3.2/4.2; unit tests
- [x] 5.3 Implement `nudge.PushNotifier`: short per-`Kind` title/body (`KindFollowUp`/`KindInterviewPrep`/`KindJobClosed`, shortened from the existing Telegram copy in `internal/nudge/transports.go`), deep-link data always set to the job slug, calls `pushnotify.SendToDevices`; unit tests for all three kinds
- [x] 5.4 Register `nudge.PushNotifier` in `cmd/nudge`'s `Router` unconditionally
- [x] 5.5 Integration test: each nudge kind with `push` in the account's channel set delivers to a registered device and soft-skips with none

## 6. Web UI

- [x] 6.1 Add a third "Push" chip to `web/src/lib/components/filters/AlertChannels.svelte`, following the existing `toggleEmail`/`emailSub` pattern (no linking step)
- [x] 6.2 Add a third "Push" chip to the "Deliver over" row in `web/src/lib/components/ReminderSettings.svelte`

## 7. Mobile tap-to-open deep link (`freehire-mobile`, separate repo)

- [x] 7.1 Add a notification-response listener (`Notifications.addNotificationResponseReceivedListener`) near `usePushNotifications.ts` / `src/app/_layout.tsx`: when `response.notification.request.content.data.slug` is present, `router.push('/jobs/' + slug)`
- [ ] 7.2 Manual device verification: trigger a push carrying a slug from a local/staging send and confirm the tap opens `src/app/jobs/[slug].tsx` for that slug; confirm a push with no slug just foregrounds the app

## 8. Verification

- [x] 8.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`
- [x] 8.2 `go test -tags=integration ./internal/notify/... ./internal/reminder/... ./internal/nudge/... ./internal/pushnotify/...` (needs Docker/testcontainers)
- [x] 8.3 Web: `eslint`/`tsc` clean on the two changed components
- [ ] 8.4 End-to-end manual smoke on a real device: enable the Push chip for a subscription, a reminder, and (if reproducible) a nudge; trigger each backend worker; confirm delivery and, where a single job is involved, the deep link
