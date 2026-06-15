## Context

The web app already serializes the full job-search filter state to the URL via `filtersToParams`/`filtersFromParams` (`web/src/lib/filters.svelte.ts`), and `GET /api/v1/jobs/search` (`internal/handler/search.go`) reads exactly those query params. The filter state — text query, every facet (with exclude and and-mode), visa, salary, sort — is thus already captured by one canonical query string.

The codebase has an established pattern for per-user resources: a Postgres table with `user_id`, an extracted service + narrow `Repository` (sqlc), and a thin handler (see `internal/jobtracking`, `internal/accounts`, `api_keys`). Auth is a stateless JWT in an httpOnly cookie; `RequireAuth` (cookie-only) guards browser-only management surfaces like API keys, while `RequireAuthOrKey` guards script-friendly endpoints.

This change adds saved searches as another such per-user resource.

## Goals / Non-Goals

**Goals:**
- Let a signed-in user save, list, re-apply, overwrite, and delete named filter sets, synced server-side across devices.
- Reuse the existing canonical filter query string as the stored representation — no new filter format.
- Follow the existing service + repository + thin handler pattern and response/error conventions.

**Non-Goals:**
- Job alerts / email notifications on new matches (deferred; would add scheduling, last-run state, delivery).
- AI-assisted filter generation from natural language.
- Anonymous / localStorage saved searches (auth-gated only; no client-side store, no login-time merge).
- Sharing saved searches between users.

## Decisions

### Store the canonical query string (TEXT), not structured JSON
The stored `query` is the same string `filtersToParams` produces. Applying a saved search is `filtersFromParams(new URLSearchParams(query))` then a URL commit. **Alternative considered:** a JSONB column mirroring `JobFilters`. Rejected — it duplicates the filter shape in a second place and adds serialization on both ends, while the query string is already the canonical wire format both the URL and the search API speak. Both formats share the same long-term risk (a renamed facet param could orphan old saves); the query string is the simpler choice with no extra mapping layer.

### Cookie-only `RequireAuth`
Saved searches are a browser convenience, not a scripting primitive, so the four endpoints use `RequireAuth` (cookie) — mirroring API-key management. **Alternative:** `RequireAuthOrKey` (as `/me/jobs` uses). Rejected for now; there is no script use case and cookie-only keeps the surface minimal. The seam is trivial to widen later if a CLI need appears.

### Service-layer validation; DB as backstop
Name length/blank, per-user 50 cap, and duplicate-name checks live in the `savedsearch.Service`, returning typed errors the handler maps to `400`/`409`. The DB still carries `CHECK (length(trim(name)) BETWEEN 1 AND 100)` and `UNIQUE (user_id, name)` as backstops so the invariant holds even if a future caller bypasses the service. **Rationale:** matches the project's "never persist an out-of-vocabulary value" posture (validate in code, enforce in schema).

### Active-set detection is client-side, by string comparison
"Which saved set is currently applied" is derived by comparing the current canonical query string to each saved `query` — there is no server-side "selected" state. **Rationale:** selecting a set is just applying its filters to the URL; persisting a selection would add state with no benefit and could drift from the actual URL.

### REST shape under `/api/v1/me/searches`
`GET` list, `POST` create, `PATCH /:id` (partial: name and/or query), `DELETE /:id`. Responses use the project envelope (`{"data": ...}`); errors flow through the central `ErrorHandler` (dup/cap → `409`, missing/foreign id → `404`, bad name → `400`). PATCH-partial (omitted field unchanged) mirrors the `TrackJob` nil-field convention.

## Risks / Trade-offs

- **Stored query strings can drift from the filter vocabulary** (a renamed facet param leaves an old saved set partially ineffective) → `filtersFromParams` already ignores unknown params gracefully (it only reads known facets), so a stale save degrades to a partial filter rather than an error. Acceptable for an MVP; a future migration could rewrite stored queries if a param is ever renamed.
- **Migration number collision**: `0019` may already be claimed by the concurrently-developed public-submissions change on another branch → verify the next free number at implementation time and renumber if needed before merging.
- **No UI test runner** in `web/` → frontend verified via `svelte-check` + lint + manual; backend CRUD/scoping covered by Go integration tests (testcontainers).
- **50-cap is a product guess** → enforced in one place (service), trivial to tune.

## Migration Plan

1. Add `migrations/00NN_saved_searches.sql` (verify the next free number). On a fresh volume Postgres initdb applies it; on the persistent prod DB it must be applied manually (project has no migration runner yet — known seam, same as prior changes).
2. Add sqlc queries, run `make sqlc` (or `sqlc generate` via a locally installed `sqlc@v1.31.1` if Docker is unavailable), commit generated `internal/db` code.
3. Ship backend (package + handler + routes) and frontend (store + component) together; the feature is additive and dark until the UI calls the endpoints.
4. **Rollback**: drop the table and revert the code; no other data depends on it (`ON DELETE CASCADE` only affects rows in this table).

## Open Questions

- None blocking. Per-user cap (50) and cookie-only auth are deliberate defaults that can be revisited without schema change (cap) or with a one-line route change (auth mode).
