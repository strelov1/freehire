## 1. Event source and the submitted signal

- [x] 1.1 Add `SourceAutoApply = "auto_apply"` to `internal/application/appevent/appevent.go`, including it in the `Sources` slice.
- [x] 1.2 Change `cmd/auto-apply/store.go`'s `Submit`'s `MarkJobApplied` call to pass `EventSource: appevent.SourceAutoApply` instead of `appevent.SourceSystem`.
- [x] 1.3 Update/add a `cmd/auto-apply` test asserting `Submit` writes the `applied` event with `source = 'auto_apply'`.

## 2. Migration

- [x] 2.1 Add migration `0144_application_nudges_auto_apply_kinds.sql` widening `application_nudges_kind_check` to add `auto_apply_submitted`, `auto_apply_blocked`, `auto_apply_failed`.
- [x] 2.2 Run `pnpm check:sql` (squawk) over the new migration.

## 3. MATCH queries

- [x] 3.1 Add `ListAutoApplySubmittedCandidates` to `internal/platform/db/queries/nudges.sql` (application_events, `kind='applied' AND source='auto_apply'`, windowed on `occurred_at`).
- [x] 3.2 Add `ListAutoApplyBlockedCandidates` (auto_apply_queue, `blocked_at IS NOT NULL`, windowed on `blocked_at`).
- [x] 3.3 Add `ListAutoApplyFailedCandidates` (auto_apply_queue, `failed_at IS NOT NULL`, windowed on `failed_at`).
- [x] 3.4 Run `make sqlc` and commit the generated `internal/platform/db` diff.

## 4. Engine wiring (`internal/engage/nudge`)

- [x] 4.1 Add `KindAutoApplySubmitted`, `KindAutoApplyBlocked`, `KindAutoApplyFailed` constants in `nudge.go`.
- [x] 4.2 Add the three new methods to the `Store` interface; add `AutoApplyOutcomeWindowDays` to `Config` and `DefaultConfig`.
- [x] 4.3 Add three MATCH loops in `Runner.match`, mirroring the existing `KindInterviewPrep` loop shape, each calling `RecordNudge` with `EpisodeKey` set to the row's own `occurred_at`/`blocked_at`/`failed_at`.
- [x] 4.4 Add the three new cases to `Runner.actionable`, each returning `true` unconditionally (the `NotificationsEnabled` gate ahead of the switch already applies).
- [x] 4.5 Confirm `GetNudgeForDelivery` needs no change (verify by reading it against the new kinds' needs — title/company/slug/url/channels/quiet-hours are already generic).

## 5. Rendering

- [x] 5.1 Add the three new cases to `push.go`'s `renderNudgeBatch` and `renderNudge` (title/body copy).
- [x] 5.2 Add the three new cases to `transports.go`'s `batchHeadline`, `renderOne`/`render` (Telegram + email single/batch bodies), and `batchCopy` (email subject/heading/preheader/lead).
- [x] 5.3 Confirm `batchDestination` needs no new case (falls through to the existing `/my/tracking` default).

## 6. Tests

- [x] 6.1 `nudge_test.go`: table-driven `actionable()` cases for the three new kinds — true regardless of job/application state, false when `NotificationsEnabled` is false.
- [x] 6.2 `transports_test.go`: one rendering case per new kind, single- and multi-job batch.
- [x] 6.3 `push_test.go`: one rendering case per new kind, single- and multi-job batch.
- [x] 6.4 `internal/platform/db` integration test (mirroring `notification_center_integration_test.go`'s pattern): each new `List*Candidates` query returns the right rows and excludes the wrong ones (e.g. `source='user'` excluded from `ListAutoApplySubmittedCandidates`; an unset `blocked_at`/`failed_at` row excluded from the other two).
- [x] 6.5 `internal/platform/db` integration test: `RecordNudge`'s `ON CONFLICT DO NOTHING` is idempotent across two MATCH passes over the same row for each of the three new kinds.

## 7. Verification

- [x] 7.1 `gofmt -l .`, `go vet ./...`, `go test ./...`.
- [x] 7.2 `go vet -tags=integration ./...`; run the full tagged suite for `internal/engage/nudge`, `internal/platform/db`, and `cmd/auto-apply`.
- [x] 7.3 `golangci-lint run`.
