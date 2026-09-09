## 1. SQL layer

- [x] 1.1 In `internal/platform/db/queries/mentorship.sql`, change `WithdrawMentorProfile` to `UPDATE mentors SET status = 'withdrawn', updated_at = now() WHERE user_id = $1` (drop the `paused = true` write and the `status <> 'withdrawn'` guard).
- [x] 1.2 Add a new query `ReactivateMentorProfile`: `UPDATE mentors SET status = 'pending', paused = false, updated_at = now() WHERE user_id = $1 AND status = 'withdrawn' RETURNING *`.
- [x] 1.3 Run `make sqlc` and confirm `internal/platform/db` regenerates with no manual edits needed.

## 2. Domain service and repository

- [x] 2.1 Add `ErrProfileNotWithdrawn` sentinel next to `ErrProfileNotPending` in `internal/engage/mentorship/profile.go`, with the same comment style (what it means, what HTTP status it maps to).
- [x] 2.2 Update `QueriesRepository.WithdrawProfile` (`repository.go`): no more 0-rows-means-not-found special case tied to "already withdrawn" — rows==0 still means `ErrProfileNotFound` (now only reachable via a genuine race, since `Service.Withdraw` already checked `ownProfile`).
- [x] 2.3 Add `QueriesRepository.ReactivateProfile(ctx, userID) (Profile, error)`: on 0 rows, call `ownProfile`-equivalent lookup to distinguish "no profile" (`ErrProfileNotFound`) from "profile exists but not withdrawn" (`ErrProfileNotWithdrawn`).
- [x] 2.4 Add `Service.Reactivate(ctx, userID) (Profile, error)` in `profile.go`, following the shape of `SetPaused`.
- [x] 2.5 Update `fakeRepo` in `fake_repo_test.go`: `WithdrawProfile` becomes idempotent (no longer errors on a second call), add `ReactivateProfile` with the same status-guard semantics.

## 3. Tests (test-first per task)

- [x] 3.1 `profile_test.go`: withdrawing an already-withdrawn profile succeeds (no error), and does not flip `paused` on an initial withdrawal.
- [x] 3.2 `profile_test.go`: withdrawing with no profile at all still returns `ErrProfileNotFound`.
- [x] 3.3 `profile_test.go`: `Reactivate` on a withdrawn profile returns it with `status == StatusPending` and `Paused == false`.
- [x] 3.4 `profile_test.go`: `Reactivate` on a pending/rejected/approved profile returns `ErrProfileNotWithdrawn`; on no profile returns `ErrProfileNotFound`; on a stranger's profile returns `ErrProfileNotFound` (owner check, matching `UpdateProfile`/`SetPaused` convention).
- [x] 3.5 `internal/platform/db/mentorship_integration_test.go`: exercise `WithdrawMentorProfile` (idempotent, doesn't touch `paused`) and `ReactivateMentorProfile` (status guard) against real Postgres.

## 4. HTTP layer

- [x] 4.1 Add `case errors.Is(err, mentorship.ErrProfileNotWithdrawn): return fiber.NewError(fiber.StatusConflict, "this profile is not withdrawn")` to `mentorshipError` in `internal/api/handler/mentorship.go`.
- [x] 4.2 Add `POST /me/mentorship/profile/reactivate` route + handler `ReactivateMentorProfile` in `internal/api/handler/mentorship_write.go`, mirroring `PauseMentorProfile`.
- [x] 4.3 Integration test in `internal/api/handler` (the 78-file suite `mentorship*.go` belongs to) covering: reactivate success, reactivate-when-not-withdrawn (409), repeat withdraw (200/204, not 404).

## 5. Frontend

- [x] 5.1 `web/src/lib/api.ts`: add `reactivateMentorProfile()` calling `POST /api/v1/me/mentorship/profile/reactivate`.
- [x] 5.2 `web/src/lib/components/MentorProfileEditor.svelte`: fix the badge to show `paused` only when `status === 'approved'`, otherwise show `status` directly.
- [x] 5.3 `MentorProfileEditor.svelte`: when `status === 'withdrawn'`, replace the "Withdraw profile" action with "Submit for review again" wired to `reactivateMentorProfile()`, and refresh the profile in place on success. Also fixed `withdraw()` to keep the withdrawn profile in local state instead of nulling it — nulling made the form immediately offer "Submit for review" (POST), which would 409 against the still-occupied one-profile slot before any reload.
- [x] 5.4 Manually verify in the browser: withdraw → badge reads "withdrawn" (not "paused") → click "Submit for review again" → badge reads "pending" → profile appears in the admin moderation queue. Verified live via `make up` + Playwright against the containerized app: badge correctly reads "withdrawn" after withdrawal, a second `DELETE` returns 204 (not 404), and "Submit for review again" moves it to "pending" with the ordinary edit/withdraw UI restored.

## 6. Docs

- [x] 6.1 Update `internal/engage/mentorship/AGENTS.md`: the "a mentor who has left" / one-way framing around `StatusWithdrawn` and `Withdraw` needs a line noting resubmission is now possible via `Reactivate`.
- [x] 6.2 Update `internal/api/handler/mentorship.go`'s `mentorshipError` doc comment set if it enumerates error cases elsewhere (check for a list to keep in sync). No other list exists (checked `web/static/openapi.yaml` and `docs/`) — the switch case plus the sentinel's own doc comment in `profile.go` are the only place this is recorded, per the file's own convention.

## 7. Verification

- [x] 7.1 `gofmt -l .` clean on touched Go files.
- [x] 7.2 `go build ./... && go vet ./...`
- [x] 7.3 `go test ./...` (212 packages, all green)
- [x] 7.4 `go vet -tags=integration ./...`
- [x] 7.5 `go test -tags=integration ./internal/platform/db/ ./internal/api/handler/ ./internal/engage/mentorship/...` (Docker/testcontainers required) — all green
- [x] 7.6 `openspec validate --strict mentorship-withdraw-reactivation`

## 8. Review

- [x] 8.1 Single whole-change code review (dispatched subagent): no Critical or Important findings. Two Minor polish items fixed — `StatusWithdrawn`'s doc comment now mentions `Reactivate`, and `save()` in `MentorProfileEditor.svelte` now notes it stays reachable while withdrawn (intentional). Re-verified `go build`/`go test`/`svelte-check` after the fixes.
