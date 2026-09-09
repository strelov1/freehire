## 1. Backend: expose submission date

- [x] 1.1 Add `CreatedAt time.Time` (JSON `created_at`, `omitempty` via zero-check or a pointer) to `mentorResponse` in `internal/api/handler/mentorship.go`.
- [x] 1.2 Set it in `toModeratorMentorResponse` and `toOwnMentorResponse`; leave `toMentorResponse` (public) untouched.

## 2. Backend: moderator photo route

- [x] 2.1 Add `Service.ProfileForModeration(ctx, id int64) (Profile, error)` in `internal/engage/mentorship/profile.go`, wrapping `repo.ProfileByID` (0 rows → `ErrProfileNotFound`).
- [x] 2.2 Add handler `GetPendingMentorPhoto` in `internal/api/handler/mentorship.go` (or `mentorship_write.go`, matching where the other moderator handlers live): resolve via `ProfileForModeration`, reuse the existing `mentorPhoto` helper, mirror `GetMentorPhoto`'s response handling.
- [x] 2.3 Register `GET /mentorship/profiles/:id/photo` behind `mw.key, mw.moderator` in `internal/api/handler/mentorship.go`'s `register`.

## 3. Tests (test-first per task)

- [x] 3.1 `internal/engage/mentorship/profile_test.go`: `ProfileForModeration` returns a pending profile by id regardless of status; returns `ErrProfileNotFound` for an unknown id.
- [x] 3.2 `internal/api/handler` integration test: moderator photo route serves a pending, opted-in mentor's photo (200); 404 for a mentor with `show_photo=false`; 404 for an unknown id; 401/403 for a non-moderator caller.
- [x] 3.3 Confirm (via an existing or new test) that `GET /api/v1/mentors/:slug` and `GET /api/v1/mentors/:slug/photo` are unaffected — still 404 for a pending/rejected profile. Covered by the existing, unmodified `TestOnlyApprovedUnpausedMentorsArePublished` (`internal/platform/db/mentorship_integration_test.go`) — re-ran it green; `GetMentor`/`GetMentorPhoto`/`PublicProfile`/`PublishedProfileBySlug` were not touched by this change.

## 4. Frontend

- [x] 4.1 `web/src/lib/types.ts`: extend `PendingMentorProfile` with `created_at`. No new api.ts function for the photo URL — the existing public page inlines its photo URL template directly (`web/src/routes/mentors/[slug]/+page.svelte:24`) rather than going through an api.ts helper, so the moderator preview follows the same convention: `/api/v1/mentorship/profiles/${profile.id}/photo` inline in the `<img>` src.
- [x] 4.2 `web/src/lib/components/MentorReviewView.svelte`: show each card's submission date (via the existing `formatDate` from `$lib/utils`, matching the convention other `created_at` displays already use).
- [x] 4.3 Add a "Preview" toggle per card that renders a public-card-style block (photo when `show_photo`, name, headline @ company, topics/languages chips, bio) from the already-fetched `PendingMentorProfile`, fetching only the photo. A photo load error hides the image (matches the public card's own `show_photo` error-hiding rule).
- [x] 4.4 Manually verify in the browser: submit a profile with a photo opted in, open the moderation queue, confirm the submission date shows and the preview renders the photo and matching fields. Verified live via `make up` + Playwright against the containerized app: "Submitted Sep 9, 2026" shows in the card header, "Preview" expands a block with the uploaded photo (served through the new moderator-only route), name, headline @ company, topics/languages, and bio.

## 5. Verification

- [x] 5.1 `gofmt -l .` clean on touched Go files.
- [x] 5.2 `go build ./... && go vet ./...`
- [x] 5.3 `go test ./...`
- [x] 5.4 `go vet -tags=integration ./...`
- [x] 5.5 `go test -tags=integration ./internal/api/handler/ ./internal/engage/mentorship/...`
- [x] 5.6 `golangci-lint run --new-from-merge-base=origin/main` — clean after fixing bodyclose in the new integration test (same wrapper-hides-Close pitfall as the previous change; fixed the same way, explicit close at each call site).
- [x] 5.7 `pnpm run check` (web) — 0 errors.
- [x] 5.8 `openspec validate --strict mentor-moderation-preview`
