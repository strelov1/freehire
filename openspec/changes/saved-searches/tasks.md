## 1. Data layer

- [x] 1.1 Verify the next free migration number (→ **0021**; 0019 job_submissions, 0020 job_reports both merged to main during this work), then add `migrations/0021_saved_searches.sql` creating `saved_searches` (id, user_id FK→users ON DELETE CASCADE, name TEXT with `CHECK (length(trim(name)) BETWEEN 1 AND 100)`, query TEXT NOT NULL, created_at/updated_at, `UNIQUE (user_id, name)`, index on `(user_id, updated_at DESC)`)
- [x] 1.2 Add `internal/db/queries/saved_searches.sql` with user-scoped queries: ListSavedSearches, CountSavedSearches, CreateSavedSearch, UpdateSavedSearch (partial: COALESCE name/query via sqlc.narg), DeleteSavedSearch — all filtering by `user_id`
- [x] 1.3 Run `make sqlc` (Docker), commit the regenerated `internal/db` code; `go build ./...` passes

## 2. Service layer (`internal/savedsearch`)

- [x] 2.1 RED: unit tests for `Service` validation — blank/whitespace name → error, name >100 chars → error, valid name passes, trim applied (also empty-query-allowed, cap, dup-name, partial update, not-found)
- [x] 2.2 GREEN: implement `savedsearch.Service` + narrow `Repository` interface (List/Count/Create/Update/Delete) over `*db.Queries`; typed errors for invalid-name, duplicate-name, cap-exceeded, not-found; enforce the 50-per-user cap in Create
- [x] 2.3 REFACTOR + simplify pass; unit tests stay green (gofmt/vet clean; rune-count name fix added via TDD for Cyrillic names)

## 3. HTTP handler + routes

- [x] 3.1 RED: integration tests (`//go:build integration`, testcontainers) in `internal/handler/me_searches_integration_test.go` — create/list/update/delete happy paths, dup-name→409, cap(50)→409, blank/over-long name→400, another user's id→404, unauthenticated→401, empty query allowed
- [x] 3.2 GREEN: implement `internal/handler/me_searches.go` (Create/List/Update/Delete) returning the `{"data": ...}` envelope; map service errors to fiber status via `savedSearchError` → central ErrorHandler
- [x] 3.3 Wire the four routes under `RequireAuth` (cookie-only) in `internal/handler/handler.go`: `GET/POST /me/searches`, `PATCH/DELETE /me/searches/:id`; integration tests recompiled and green
- [x] 3.4 REFACTOR + simplify pass; tests stay green (multi-agent code review: no Critical/Important; isUniqueViolation dup noted as a seam, not extracted to avoid touching unrelated packages)

## 4. Web store

- [x] 4.1 Add `web/src/lib/savedSearches.svelte.ts`: a reactive class store ($state.raw) that loads the list via `GET /api/v1/me/searches` and exposes create/update/delete; `SavedSearch` type added to types.ts; 4 API fns added to api.ts. Also added `FilterStore.apply(query)` + `canonicalQuery()` to filters.svelte.ts
- [x] 4.2 Active-set derivation (`activeId` via `canonicalQuery` compare) lives in the component (has the FilterStore); `base`/`dirty` derivations drive the "Update" affordance

## 5. Web UI

- [x] 5.1 Add `web/src/lib/components/SavedSearches.svelte`: "My filters" `<select>` (apply via `store.apply` + URL commit; value = active set), inline name input to save; "Update '<name>'" shown when a base set is selected and filters changed (dirty); "Delete" when the active set is exactly applied
- [x] 5.2 Signed-out state: "Sign in to save filters" button opens the auth dialog via `openAuthDialog()` (gated on `isAuthenticated()`, the project's auth signal)
- [x] 5.3 Mount `SavedSearches` at the top of `FiltersPanel.svelte`, passing the existing `FilterStore` (rides into both desktop sidebar and mobile drawer)
- [x] 5.4 `svelte-check` 0 errors/0 warnings; eslint clean on changed files (project lint baseline is pre-red; reverted an accidental ad-hoc prettier reformat that diverged quote style)

## 6. Verification

- [x] 6.1 `go build ./... && go vet ./...`; `go test ./...`; `go test -tags=integration ./internal/handler/` green (full handler suite 174s, all pass); web `svelte-check` 0/0, `npm run build` SSR-builds clean
- [x] 6.2 Manual end-to-end in a real browser (agent-browser, live stack on :8095/:5173 against hire-db-1 with the migration applied): signed-out shows "Sign in to save filters" → opens auth dialog; signed in, applied Remote+Europe → saved "Remote EU" (persisted, appears in picker); Reset + re-select → filters re-applied; edited (add Hybrid) → "Update 'Remote EU'" appeared → updated (server query changed, button cleared); deleted (set gone, server empty). 🔍 probe: duplicate name → inline error "a saved search with this name already exists", no extra row. Screenshots captured.
- [x] 6.3 `openspec validate saved-searches --strict` passes
