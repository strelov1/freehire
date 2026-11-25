## 1. Dependency

- [x] 1.1 Add `github.com/disintegration/imaging` to `go.mod`/`go.sum` (`go get
      github.com/disintegration/imaging`).

## 2. Blur primitive (`internal/candidate/headshot`)

- [x] 2.1 Write a failing test in `internal/candidate/headshot/blur_test.go`:
      `Blur` on a valid stored-shape JPEG (reuse a fixture from
      `normalize_test.go` if one exists, or a small generated test image) returns a
      byte slice that (a) decodes as a valid JPEG of the same dimensions, and (b) is
      NOT byte-identical to the input.
- [x] 2.2 Write `internal/candidate/headshot/blur.go`: `func Blur(data []byte) ([]byte,
      error)` — decode via `image.Decode`, `imaging.Blur(img, blurSigma)` with an
      unexported `const blurSigma = 30`, re-encode via `jpeg.Encode(...,
      &jpeg.Options{Quality: jpegQuality})` reusing the existing package constant.
      Wrap decode/encode errors with context, matching `normalize.go`'s error style.
- [x] 2.3 Confirm the test passes; add a second test case asserting the blur is REAL,
      not just a re-encode. Implemented as an edge-softening assertion instead of the
      originally sketched local-variance measurement: a pixel sitting exactly on a
      hard band boundary in a synthetic input must come out carrying a visible blend
      of both neighbouring colors after blurring — a more direct, less flaky proof of
      blurring than a variance ceiling. `TestBlur_SoftensASharpEdge`, passing.

## 3. Membership → user id lookup (`internal/candidate/talentnetwork`)

- [x] 3.1 Add `GetTalentNetworkMemberUserIDByHandle` to
      `internal/platform/db/queries/users.sql`: `SELECT u.id FROM users u WHERE
      u.talent_handle = sqlc.arg(handle)::text AND u.talent_network_visibility <>
      'off' AND u.resume_uploaded_at IS NOT NULL AND u.resume_structured_uploaded_at =
      u.resume_uploaded_at` — the exact predicate `GetTalentNetworkMemberByHandle`
      uses, minus the `user_profiles` join and the extra selected columns this lookup
      doesn't need. Run `make sqlc` to regenerate.
- [x] 3.2 Write a failing test in `internal/candidate/talentnetwork/catalogue_test.go`:
      `HeadshotOwner` returns the correct user id for a current member's handle, and
      `ErrNotFound` for an absent/malformed handle (the three DB-side absence reasons
      collapse into "the store returns no row," so they're one test case, matching
      `TestByHandle_AbsentAndMalformedAnswerTheSame`'s own shape) plus a dedicated
      no-query check for a malformed handle.
- [x] 3.3 Add `HeadshotOwner(ctx context.Context, handle string) (int64, error)` to
      `Catalogue` in `internal/candidate/talentnetwork/catalogue.go`, calling the new
      query directly against `c.store` (same `ValidHandle` pre-check `ByHandle` does,
      same `pgx.ErrNoRows` → `ErrNotFound` mapping). Tests pass.

## 4. Public route (`internal/api/handler`)

- [x] 4.1 Write a failing test in `talent_catalog_test.go` covering `GET
      /talent/:handle/photo`: 200 with `Content-Type: image/jpeg` and a body that is
      NOT byte-identical to the stored original for a member with a headshot; 404 for
      a member with no headshot, a non-member handle, a malformed handle, and (with a
      nil `*headshot.Store`) unconfigured storage; all four 404 bodies
      byte-identical. Implemented as a plain (non-integration-tagged) test: it builds
      a REAL `*headshot.Store` and `*talentnetwork.Catalogue` over small in-memory
      fakes of `blobstore.Store`/`headshot.Repository`/`talentnetwork.Store` — the
      same shape `talent_catalog_test.go` already uses for the card route — so it
      exercises the real membership/headshot/blur logic without needing Docker.
- [x] 4.2 Implement `GetPhoto` in `talent_catalog.go`: `HeadshotOwner` →
      `headshot.Get` → `headshot.Blur` → `image/jpeg` response with
      `X-Robots-Tag: noindex` and `Cache-Control: private, max-age=60`; every error
      path (`ErrNotFound`, `headshot.ErrNotStored`, `headshot.ErrStorageDisabled`)
      maps to the same `fiber.NewError(fiber.StatusNotFound)`.
- [x] 4.3 Register the route in `talentCatalogHandlers.register`: `api.Get(
      "/talent/:handle/photo", limiter, h.GetPhoto)`, reusing the existing `limiter`
      variable already shared by `/talent` and `/talent/facets`. Tests pass:
      `go test ./internal/api/handler/...`.
- [x] 4.4 Updated `talentCatalogHandlers` construction in `internal/api/handler/handler.go`
      to also receive `photoStore` (`*headshot.Store`), already constructed earlier in
      the same function for `photoHandlers`/`cvHandlers`/`referralHandlers`.

## 5. Frontend (`web/src/routes/talent/[handle]/+page.svelte`)

- [x] 5.1 Replaced the unconditional generic-icon `<div>` with `Avatar` from `$lib/ui`
      (`design-system/src/avatar.svelte` — the SAME component `EntityLogo` re-exports,
      already SSR-safe against the "error fires before hydration" gap the task
      described, via its own `catchMissedError` attach handler) — no name passed, so a
      failed/absent photo falls through to the `fallbackIcon` snippet, never the
      colour/initials branch. `src="/api/v1/talent/{member.handle}/photo"`,
      `size="lg" class="size-14 shrink-0 bg-secondary"` (tailwind-merge resolves the
      `size-14` override), `fallbackIcon` renders the same `User` icon as before.
      Reusing the existing primitive turned out simpler than hand-rolling the
      onerror/state logic the task sketched.
- [x] 5.2 Updated the footer disclaimer paragraph to add a sentence stating that an
      uploaded photo, when present, is shown heavily blurred and cannot be recovered
      in its original form.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean; `go vet ./...`; `go test ./...` — all green (one
      pre-existing, unrelated failure in `cmd/billing-sync`
      `TestTheStoreProviderAloneKeepsTheWorkerRunning`, reproduces in isolation on
      `main` with no files touched by this change — environment-dependent, not
      caused by this work).
- [x] 6.2 `go vet -tags=integration ./...` clean. Did not additionally run the
      Docker-based integration suite for this change specifically — task 4.1's tests
      already exercise the real `Catalogue`/`headshot.Store`/blur logic over
      in-memory fakes, matching this file's existing non-integration-tagged test
      convention, so there is no integration-only behavior left unverified.
- [x] 6.3 `pnpm --filter web check` (0 errors, 41 pre-existing unrelated warnings)
      and `pnpm --filter web lint` (clean on the changed file) both pass.
- [x] 6.4 Manually verified against a REAL running backend (`cmd/server`) + real
      Postgres + real MinIO, with two temporary seeded test accounts (deleted after
      verification, along with the uploaded test blob): a member with a stored
      headshot renders a visibly, strongly blurred photo (skin-tone/feature smudges,
      no discernible eyes/edges) in the same circular slot the icon used to occupy;
      a member with no headshot renders the exact same fallback icon as before, no
      broken-image flash. Screenshots taken via Playwright.
