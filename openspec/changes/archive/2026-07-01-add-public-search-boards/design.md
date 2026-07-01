## Context

Saved searches (`saved_searches` table, `internal/savedsearch` service, `/api/v1/me/searches` under `RequireAuth`) let a signed-in user store named filter snapshots (`query` = the canonical search string the filter URL and `GET /jobs/search` share). They are private and addressed by internal numeric id. Separately, the repo already runs a "public read by slug" pattern: `jobs.public_slug TEXT NOT NULL UNIQUE`, `GetJobBySlug`, and `api.Get("/jobs/:slug", ...)` with no auth middleware. This change grafts that pattern onto saved searches so a user can publish one as a public "board".

Design decisions were settled with the user up front:
- Slug is readable — `normalize(name)` + short random suffix — over an unguessable token.
- Author attribution is a self-contained optional `author_label` on the board, not a new `users.display_name` profile subsystem.
- Sharing is a nullable `public_slug` toggle on the existing row, not a separate `public_boards` entity.

## Goals / Non-Goals

**Goals:**
- Let any user turn one of their saved searches into a link-shareable, publicly readable board and revoke it.
- Reuse the existing public-read-by-slug pattern and the existing jobs-list rendering (the board page just applies the stored query).
- Expose zero owner PII on the public surface.
- Keep all write/management operations cookie-only, exactly like the rest of `/me/searches`.

**Non-Goals:**
- No user profile / `display_name` subsystem (author is a per-board free-text label).
- No board discovery/listing/search of others' boards, no unguessable-token privacy guarantee, no view analytics, no board-level access control beyond shared/not-shared.
- No changes to how private saved searches behave today.

## Decisions

### Slug: `normalize(name)` + short random suffix, minted at share time
Reuse `internal/normalize` (already used for job/company slugs) on the set's name, append a short random suffix (e.g. 4 lowercase base36 chars) to disambiguate and reduce guessability while keeping a readable URL: `remote-go-jobs-a3f1`. Uniqueness is enforced by the `UNIQUE` index; on the rare insert conflict, regenerate the suffix and retry a bounded number of times. Slug is minted once and kept across re-shares; unshare clears it, and a later share mints a fresh one.
- *Alternative rejected*: random opaque token (more private, less friendly) — user chose readability. Trade-off recorded under Risks.
- *Alternative rejected*: slug derived purely from id (stable but ugly and enumerable).

### Model: nullable `public_slug` + `author_label` columns on `saved_searches`
`ALTER TABLE saved_searches ADD COLUMN public_slug TEXT` with a `UNIQUE` index (partial or plain — a plain UNIQUE tolerates multiple NULLs in Postgres, so a plain `UNIQUE (public_slug)` is sufficient and simplest) and `ADD COLUMN author_label TEXT`. `public_slug IS NULL` ⇔ private. This keeps one entity and one source of truth for `query`; no cross-table sync.
- *Alternative rejected*: a separate `public_boards(slug, saved_search_id, ...)` table — duplicates `query` or forces a join, and adds a lifecycle to keep in sync, for no gain at this scope.

### Endpoints mirror existing conventions
- `POST /api/v1/me/searches/:id/share` (RequireAuth) → owner-scoped; body `{author_label?}`; returns the updated `savedSearchResponse` (now including `public_slug`, `author_label`).
- `DELETE /api/v1/me/searches/:id/share` (RequireAuth) → owner-scoped; `204`.
- `GET /api/v1/boards/:slug` (no auth) → public read; returns `{data: {name, query, author_label}}` only. Modeled on `GetJob`/`GetJobBySlug`. A missing/unshared slug returns `pgx.ErrNoRows` → the central `ErrorHandler` maps it to 404 (no per-handler remapping, per repo convention).

The service layer (`internal/savedsearch`) gains `Share(ctx, userID, id, authorLabel)`, `Unshare(ctx, userID, id)`, and `GetPublicBySlug(ctx, slug)`, plus slug generation. New sqlc queries: `SetSavedSearchPublicSlug` (owner-scoped update), `ClearSavedSearchPublicSlug` (owner-scoped), `GetPublicBoardBySlug` (reads only where `public_slug = $1`). `ListSavedSearches`/`GetSavedSearch` return the two new columns (regen from `SELECT *`).

### Wire shape and web
`savedSearchResponse` gains `public_slug` (string, empty when null) and `author_label` (string). A new public board response type carries only `name`, `query`, `author_label`. The SPA adds `web/src/routes/b/[slug]/` that fetches the board and renders the existing jobs list seeded with the board's query (reusing the same query→filter parsing the "apply a saved search" flow already uses).

**Account section placement.** The account area under `/my/*` is a flat set of sibling pages (`/my/jobs`, `/my/profiles`, `/my/notifications`, `/my/api-keys`, `/my/submissions`), each linked from the `menuItems` array in `HeaderMenu.svelte` — there is no shared `/my/+layout`. So "Saved searches" is added the same way: a new `web/src/routes/my/searches/+page.svelte` plus one entry in `HeaderMenu.svelte`. The page reuses the existing `savedSearches` store (`web/src/lib/savedSearches.svelte.ts`) for the list and the new share/unshare API client calls.

**Share/unshare UI reuse.** Both the account section and the filters "My filters" control (`SavedSearches.svelte`) call the same new `api.ts` functions (`shareSavedSearch(id, authorLabel)` → returns the updated set with slug; `unshareSavedSearch(id)`), and the shared/unshared state is read from `public_slug` on the listed set. Creation stays only in the filters control (it owns the current-filters `query`); the account section is management-only. TS contracts regen via `cmd/gen-contracts` where applicable.

## Risks / Trade-offs

- **Readable slug is partially guessable / enumerable** → Accepted per user decision; mitigated by the random suffix (not pure name) and by the invariant that a board is reachable only while explicitly shared. No PII is exposed even on a correct guess (only name/query/author_label, all author-authored).
- **Unshare breaks existing links** → This is the intended semantics of revocation; documented in the spec. Re-share mints a new slug rather than reviving the old one.
- **Slug collision on insert** → Bounded retry with a fresh suffix; `UNIQUE` index is the backstop.
- **Author label is free text (light PII by choice)** → It is optional and author-supplied; empty renders anonymously. We never derive it from `email`.
- **Migration on a persistent DB** → Repo has no versioned migration runner yet (known seam); the two `ADD COLUMN` statements are additive and nullable, safe to apply manually on prod (per existing deploy convention) before the code ships.

## Migration Plan

1. Add migration `00NN_saved_searches_public_slug.sql` (two nullable columns + `UNIQUE` index on `public_slug`); `make sqlc`.
2. Ship service + handlers + routes behind the existing conventions; no behavior change for private sets.
3. Web route + filters-panel controls.
4. Rollback: unshare is reversible; to fully back out, drop the two columns (no data loss for private saved searches, which never populate them).
