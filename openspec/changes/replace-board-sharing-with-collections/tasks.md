## 1. Schema

- [x] 1.1 Add migration creating `job_lists` (id, user_id, name, description, public_slug nullable, created_at, updated_at, UNIQUE(user_id, name)) and its partial unique index on `public_slug`
- [x] 1.2 Add migration creating `job_list_items` (collection_id/job_id composite PK, added_at), FKs to `job_lists` and `jobs` with `ON DELETE CASCADE`
- [x] 1.3 Run `make sqlc` after adding `internal/platform/db/queries/job_lists.sql` (create/list/get/update/delete list; add/remove/list items; set/clear public slug; get by slug) and commit the generated diff

## 2. Slug helper relocation

- [x] 2.1 Move the slug-minting logic (transliteration + random suffix + collision retry) out of `internal/search/savedsearch/share.go` into a new `internal/dict/slugmint` package (not `internal/platform`: the helper depends on `dict/normalize`, and platform sits below dict in the layer order, so platform cannot import it)
- [x] 2.2 Update `internal/search/savedsearch` to have no remaining reference to the moved helper (it's being deleted in section 5, not re-pointed at the new location)

## 3. `internal/application/joblists` package

- [x] 3.1 Define the service: `List`, `Create(name, description)`, `Update(id, name, description)`, `Delete(id)`, `AddJob(id, jobID)`, `RemoveJob(id, jobID)`, `Share(id) -> slug`, `Unshare(id)`, `GetPublicList(slug)`
- [x] 3.2 Enforce name validation (trim, 1-100 chars, unique per user) and description validation (trim, max 2000 chars)
- [x] 3.3 Enforce the 50-lists-per-user cap on create
- [x] 3.4 Make `AddJob`/`RemoveJob` idempotent (already-present add / absent remove are no-ops); reject an unknown `job_id` on add
- [x] 3.5 `Share` reuses an existing slug when re-sharing (uses the relocated helper from section 2 to mint on first share)
- [x] 3.6 `GetPublicList` renders jobs through the existing `jobview` projection, excludes owner-identifying fields, and 404s (returns not-found) for an unknown or unshared slug
- [x] 3.7 Unit tests for the service: name/description validation, cap, idempotent add/remove, share/unshare/re-share slug behavior, owner-scoping (cannot touch another user's list)

## 4. API routes

- [x] 4.1 `GET/POST /api/v1/me/lists`, `PATCH/DELETE /api/v1/me/lists/:id` (cookie-only, owner-scoped, 404 on cross-user access)
- [x] 4.2 `POST /api/v1/me/lists/:id/jobs` (body `{job_slug}`), `DELETE /api/v1/me/lists/:id/jobs/:job_slug` — jobs are addressed by public slug at the wire boundary (like save/unsave, tracking), resolved to the internal id via `joblists.Repository.JobIDBySlug` (mirrors `jobtracking`'s pattern)
- [x] 4.3 `POST /api/v1/me/lists/:id/share`, `DELETE /api/v1/me/lists/:id/share`
- [x] 4.4 Public `GET /api/v1/lists/:slug` (no auth), returning `{name, description, jobs}`
- [x] 4.4b `GET /api/v1/me/lists/membership?job_slug=` — every list flagged `in_list`, for the job card's "Add to list" toggle (discovered while wiring the frontend: showing/toggling per-job membership needs this, since `GET /me/lists` only carries list-level `job_count`)
- [x] 4.5 Integration-tagged handler tests covering the scenarios in `specs/saved-job-lists/spec.md` (auth required on `/me/lists/*`, 401/404/409/400 cases, public read of shared/unshared/unknown slug, closed job still listed)

## 5. Remove board sharing (backend)

- [x] 5.1 Delete `Share`/`Unshare`/`GetPublicBoard`, `share.go`, and the `Board` type from `internal/search/savedsearch`
- [x] 5.2 Delete the `GET /boards/:slug` route/handler (`internal/api/handler/boards.go`) and the share/unshare routes in `internal/api/handler/me_searches.go`
- [x] 5.3 Delete the sqlc queries `SetSavedSearchPublicSlug`, `ClearSavedSearchPublicSlug`, `GetPublicBoardBySlug` and run `make sqlc` (kept `GetSavedSearch`: still used by `internal/engage/subscription`)
- [x] 5.4 Update/remove tests that exercised the deleted share/unshare/public-board behavior
- [x] 5.5 Add migration dropping `saved_searches.public_slug`, `saved_searches.author_label`, and `saved_searches_public_slug_idx` (separate file, after the section 1 migrations land)

## 6. Frontend: job lists

- [x] 6.1 Add `api.ts` client functions and `types.ts` types for job lists (list/create/update/delete, add/remove job, share/unshare, public read) — plus `listJobListMembership`/`JobListMembership` for 4.4b
- [x] 6.2 Add the public route (`web/src/routes/l/[slug]`) mirroring `web/src/routes/b/[slug]`'s structure: `+page.server.ts` loads the list by slug, `+page.svelte` renders name/description/jobs (closed jobs shown with a closed indicator via the card's existing `closed_at`), not-found state for an unknown/unshared slug, `noindex` meta (via `Seo`'s `robots` prop)
- [x] 6.3 Add the account-area management page for job lists (`JobListsView.svelte` + `/my/lists`, registered in `accountNav.ts`/`accountNavIcons.ts`): create/rename/edit description/delete/share/unshare/copy-link, sign-in prompt for anonymous visitors. Per-job add/remove lives on the job card control instead (6.4), not here — see the spec correction in `specs/saved-job-lists/spec.md`
- [x] 6.4 Add the "Add to list" control (`AddToListButton.svelte`) on the job card (`JobRow.svelte`) and detail page (`JobView.svelte`'s action strip): lazily fetches membership on first open (4.4b), toggles add/remove per list, inline create-and-add, sign-in prompt when anonymous

## 7. Frontend: remove board sharing

- [x] 7.1 Delete `web/src/routes/b/[slug]`
- [x] 7.2 Remove the share/unshare/copy-link UI from `SavedSearchesView.svelte`
- [x] 7.3 Remove the now-unused `shareSavedSearch`/`unshareSavedSearch`/`getBoard` functions from `api.ts` and the `Board` type from `types.ts`; also updated `web/src/lib/docs/api-spec.ts`'s public API docs page (removed the board-share entries, documented `/lists/{slug}` and the new `/me/lists/*` endpoints)
- [x] 7.4 Remove `public_slug`/`author_label` from the `SavedSearch` type and `savedSearches.svelte.ts`'s share/unshare methods; fixed the resulting fixture in `saveSearchAlert.test.ts`

## 8. Verification

- [x] 8.1 `gofmt -w`, `go vet ./...`, `go test ./...` pass
- [x] 8.2 `go vet -tags=integration ./...` passes; ran the tagged suite for `internal/application/joblists` (via `internal/api/handler`'s `TestJobListsEndToEnd`), `internal/api/handler` (full), `internal/search/savedsearch` (unit) — all green
- [x] 8.3 Manually verified against the full `docker compose up` stack (custom host ports to avoid colliding with other concurrent worktree stacks on the shared machine) driven by a real headless-Chromium session (Playwright) with an injected session cookie: created a list, added a job from its card's "Add to list" popover (confirmed job_count updates on `/my/lists` — the store-sync fix), shared it, opened `/l/:slug` unauthenticated in a fresh browser context (renders name/description/job), confirmed the old `/b/:slug` 404s, unshared (public link then 404s), removed the job via the same card control (count back to 0), and confirmed `/my/notifications/searches` carries no leftover share/unshare/copy-link UI. Screenshots taken at every step.
- [x] 8.4 `openspec validate replace-board-sharing-with-collections --type change --strict` passes

## 9. Code-review fixes

A `code-review high` pass over the full diff found four issues, all fixed:

- [x] 9.1 `fromRow()` never set `JobCount`, so `Update`/`Share` responses always carried `job_count: 0` regardless of real membership. Fixed by adding a `job_count` subquery to `UpdateJobList`/`SetJobListPublicSlug` (matching `ListJobLists`') and building the domain `JobList` from the new row shapes directly in `repository.go` instead of the shared `fromRow`. Covered by new integration-test assertions (rename and share of a non-empty list both check `job_count`).
- [x] 9.2 `AddToListButton.svelte`'s toggle/create called `api.addJobToList`/`removeJobFromList`/`createJobList` directly instead of the `jobLists` store's mutators, so `/my/lists`' cached list (if already loaded in the same session) went stale after a card-driven add/remove/create — and `jobLists.addJob`/`removeJob` were consequently dead code. Fixed by routing through `jobLists.addJob`/`removeJob`/`create`.
- [x] 9.3 The job-lists integration test registered the remove-job route as `/me/lists/:id/jobs/:job_id` while the handler reads `c.Params("job_slug")` (production uses `:job_slug`), so the "idempotent removal" assertions passed unconditionally without exercising real removal. Fixed the route registration and added real assertions (job_count and membership both checked after the real remove, not just the status code).
- [x] 9.4 No per-list job cap meant the public, unauthenticated `GetPublicList` read (which renders every job's blurb via a `regexp_replace` over the TOASTed `description` column) had unbounded per-request cost, unlike the paginated board-sharing path it replaces. Added a 200-job-per-list cap (`ErrListFull`, mapped to `409`), exempting idempotent re-adds of an existing member; new unit tests (`TestAddJob_RejectsPastPerListCap`, `TestAddJob_ExemptFromCapWhenAlreadyAMember`) and a new `saved-job-lists` spec requirement ("Per-list job cap").
