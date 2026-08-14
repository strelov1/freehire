## 1. Migration & generated queries

- [x] 1.1 Migration: `saved_searches.derived_from_profile boolean NOT NULL DEFAULT false`; `CREATE UNIQUE INDEX saved_searches_derived_from_profile_idx ON saved_searches (user_id) WHERE derived_from_profile` (partial index enforces at most one per user).
- [x] 1.2 `internal/db/queries/saved_searches.sql`: `CreateSavedSearch` gains the `derived_from_profile` column/param; `SELECT *`-based queries (`ListSavedSearches`, `UpdateSavedSearch`, `GetSavedSearch`) pick the new column up automatically once regenerated.
- [x] 1.3 `make sqlc`; verify generated code compiles.

## 2. `internal/pgerr`: distinguish which unique constraint fired

- [x] 2.1 Add `pgerr.UniqueViolationConstraint(err) (name string, ok bool)` — returns the violated constraint's name (from `pgconn.PgError.ConstraintName`) when `err` is a unique violation, ok=false otherwise. Unit test covering: non-unique-violation error, wrapped unique violation, constraint name extraction.

## 3. `internal/savedsearch`

- [x] 3.1 `Repository.Create` signature gains `derivedFromProfile bool`; `QueriesRepository.Create` passes it through and uses `pgerr.UniqueViolationConstraint` to map a hit on `saved_searches_derived_from_profile_idx` to a new `ErrProfileSearchExists` (409) instead of the existing `ErrDuplicateName` mapping (which stays for the name-uniqueness constraint).
- [x] 3.2 `Service.Create` signature gains `derivedFromProfile bool`, passed through to the repository (no new validation beyond the existing name/cap checks — the constraint does the invariant work).
- [x] 3.3 `SavedSearch` domain struct gains `DerivedFromProfile bool`; `fromRow` maps it.
- [x] 3.4 Unit tests (fake repository): `Create` with the flag set stores it; a second `Create` with the flag set surfaces `ErrProfileSearchExists`; existing `ErrDuplicateName`/`ErrCapExceeded` paths unaffected.

## 4. Backend API

- [x] 4.1 `internal/handler/me_searches.go`: `createSavedSearchRequest` gains `DerivedFromProfile bool` (json `derived_from_profile`, default false — every existing caller unaffected); `savedSearchResponse`/`toSavedSearchResponse` include it; `savedSearchError` maps `ErrProfileSearchExists` to 409.
- [x] 4.2 Unit tests: create with the flag round-trips in the response; a second flagged create 409s; existing tests (unflagged create/update/delete) unaffected.
- [x] 4.3 Integration test (`//go:build integration`): the partial unique index actually rejects a second `derived_from_profile=true` row for the same user against real Postgres.

## 5. Web: toggle + sync

- [x] 5.1 `web/src/lib/types.ts`: `SavedSearch` gains `derived_from_profile: boolean`.
- [x] 5.2 `web/src/lib/api.ts`: `createSavedSearch` accepts an optional `derived_from_profile` param (default omitted/false); no other client signature changes (`updateSavedSearch`/`deleteSavedSearch` already take what's needed).
- [x] 5.3 New component (or a small addition to an existing profile-settings card, matching `AccountTimezone.svelte`'s autosave-card style): reads `savedSearches.items` for an existing `derived_from_profile` row to determine on/off; enable computes `filtersFromProfile(profile)` → `toSearchString()` → create (flagged) → subscribe default channel (`notification_settings.channels`, falling back to `['email']`); disable deletes the row.
- [x] 5.4 `ProfileForm.svelte`'s `onSaved` path (or the page-level `handleSaved` in `/my/profile/+page.svelte`): after a successful profile save, if a `derived_from_profile` search exists, recompute its query and `PATCH` it — fire-and-forget, best-effort (log-only on failure, never blocks/rolls back the profile save).
- [x] 5.5 Wire the toggle into `/my/notifications/searches` (`SavedSearchesView.svelte`, after the Telegram card, gated on a candidate profile existing) — relocated from `/my/profile`'s Settings tab per post-ship user feedback; see design.md.
- [x] 5.6 Unit tests for the sync/enable/disable logic (`web/src/lib/*.test.ts`, DB-free — fake API client).

## 6. Verification

- [x] 6.1 `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, `go vet -tags=integration ./...` clean.
- [x] 6.2 `go test -tags=integration ./...` (full module) clean.
- [x] 6.3 Web `eslint`/`svelte-check`/`pnpm test` clean on changed/new files.
- [x] 6.4 Manual smoke against a local backend+DB: enable the toggle, confirm a saved search + subscription appear in `/my/notifications/searches`; edit the profile, confirm the search's query changes; disable the toggle, confirm the search disappears.
