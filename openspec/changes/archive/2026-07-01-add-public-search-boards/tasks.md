## 1. Schema & generated DB access

- [x] 1.1 Add migration `migrations/00NN_saved_searches_public_slug.sql`: `ALTER TABLE saved_searches ADD COLUMN public_slug TEXT`, `ADD COLUMN author_label TEXT`, and a `UNIQUE` index on `public_slug`.
- [x] 1.2 Add sqlc queries in `internal/db/queries/saved_searches.sql`: `GetSavedSearch` (owner-scoped read), `SetSavedSearchPublicSlug` (owner-scoped set of slug + author_label), `ClearSavedSearchPublicSlug` (owner-scoped, slug→NULL), `GetPublicBoardBySlug` (read only where `public_slug = $1`); list/get select the new columns.
- [x] 1.3 Run `make sqlc` and confirm `internal/db` regenerates with the new columns and queries (build passes).

## 2. Service layer (`internal/savedsearch`)

- [x] 2.1 Add slug generation: `normalize(name)` + short random suffix, with bounded retry on unique-collision. Unit-test collision retry and readable-slug shape.
- [x] 2.2 Add `Share(ctx, userID, id, authorLabel)`: validates/trims author_label (≤60), mints slug when absent (keeps existing on re-share), applies label; owner-scoped (missing/non-owned → ErrNotFound). Unit-test share, re-share-keeps-slug, over-long label, not-owned.
- [x] 2.3 Add `Unshare(ctx, userID, id)`: clears slug, owner-scoped, idempotent no-op when already private; non-owned → ErrNotFound. Unit-test.
- [x] 2.4 Add `GetPublicBySlug(ctx, slug)`: returns name/query/author_label for a currently-shared board; unknown/unshared → ErrNotFound (or `pgx.ErrNoRows`). Unit-test found and not-found.

## 3. HTTP handlers & routes

- [x] 3.1 Extend `savedSearchResponse` with `public_slug` (empty when NULL) and `author_label`; update `toSavedSearchResponse`. Adjust list/update/create responses.
- [x] 3.2 Add `ShareSavedSearch` (`POST /me/searches/:id/share`) and `UnshareSavedSearch` (`DELETE /me/searches/:id/share`) in `internal/handler/me_searches.go`, mapping service sentinels via `savedSearchError`; both under `RequireAuth`. Handler tests (integration build-tag) for share/unshare/not-owned/401/over-long-label.
- [x] 3.3 Add public boards handler `GetBoard` (`GET /api/v1/boards/:slug`) returning `{data: {name, query, author_label}}` only, no owner fields; unknown/unshared → 404 via central ErrorHandler. Handler test for read + 404.
- [x] 3.4 Register the three routes in `internal/handler/handler.go` (two under the `/me/searches` auth group, one public alongside `/jobs/:slug`).

## 4. Web (SPA)

- [x] 4.1 Regenerate/extend the TS contract + `SavedSearch` type (public_slug, author_label) and add a board wire type; add `shareSavedSearch(id, authorLabel)` and `unshareSavedSearch(id)` to `web/src/lib/api.ts` following the existing saved-search client pattern.
- [x] 4.2 Add public route `web/src/routes/b/[slug]/` that fetches `GET /api/v1/boards/:slug`, renders board name + author label, and lists jobs by applying the stored query (reuse the existing query→filter parsing); render a not-found state for an unknown slug.
- [x] 4.3 Add the account section `web/src/routes/my/searches/+page.svelte` (list via the `savedSearches` store; share/unshare/rename/delete; copy `/b/:slug` link when shared; sign-in prompt for anonymous) and add a "Saved searches" entry to `menuItems` in `HeaderMenu.svelte`.
- [x] 4.4 Add share/unshare controls to the "My filters" panel (`SavedSearches.svelte`): share (optional author label) surfaces a copyable `/b/:slug` link; unshare reverts to private. Reflect shared state from `public_slug`.

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./...`, `go test ./...`, and the integration-tagged handler/db tests pass; web `svelte-check` clean (0 errors/0 warnings) and production SSR `npm run build` succeeds.
- [x] 5.2 End-to-end verified at the API level by `TestSavedSearchBoardsEndToEnd` against a real Postgres: create → share → open `/api/v1/boards/:slug` anonymously (no owner PII: user_id/id/email asserted absent) → re-share keeps slug → unshare → slug 404s → unknown slug 404s. Residual (not run here): a browser walkthrough of `/my/searches` and `/b/:slug` against a seeded Docker stack — recommended smoke before deploy.
