## Why

Job seekers reuse the same filter combinations every visit (e.g. "remote backend in EU, Go, senior"), but today the only way to recall a filter set is to bookmark a URL or rebuild it by hand. A signed-in user should be able to name a filter set once and re-apply it in one click from any device.

## What Changes

- Add a per-user **saved searches** capability: a signed-in user can save the current job-search filter state under a name, list their saved sets, re-apply one, overwrite ("update") one, and delete one.
- A saved search stores the **canonical search query string** — exactly what the web filter layer already serializes to the URL and what `GET /api/v1/jobs/search` already reads. No new filter format is introduced; the whole filter state (text query, all facets incl. exclude/and-mode, visa, salary, sort) is captured as one snapshot.
- New REST surface under `/api/v1/me/searches` (cookie-only auth, like API-key management — this is a browser feature).
- New Postgres table `saved_searches` (per-user, name-unique, capped at 50 per user).
- Web UI: a "My filters" control at the top of the filters panel to select / save / update / delete saved sets; anonymous users see a "sign in to save" prompt instead.
- Out of scope (explicitly deferred): job alerts / email notifications, AI-assisted filter generation.

## Capabilities

### New Capabilities
- `saved-searches`: per-user CRUD over named job-search filter snapshots — the `saved_searches` table, the `/api/v1/me/searches` API (cookie-only), the service-layer validation (name 1–100 chars, unique per user, 50-set cap), and the web "My filters" select/save/update/delete UI.

### Modified Capabilities
<!-- None: this is additive. The job-search filter wire format is reused as-is, not changed. -->

## Impact

- **Database**: new migration adding `saved_searches` (FK to `users`, `ON DELETE CASCADE`).
- **Backend**: new `internal/savedsearch` package (Service + Repository), new `internal/handler/me_searches.go`, new sqlc queries (`internal/db/queries/saved_searches.sql`) + regenerated `internal/db`, four new routes wired in `internal/handler/handler.go`.
- **Frontend**: new `web/src/lib/savedSearches.svelte.ts` store and `web/src/lib/components/SavedSearches.svelte`, mounted in `FiltersPanel.svelte`; reuses existing `filtersToParams`/`filtersFromParams`.
- **Auth**: reuses the existing cookie session (`RequireAuth`); no new auth mechanism.
- **No breaking changes**: existing job-search, filter URL behavior, and APIs are untouched.
