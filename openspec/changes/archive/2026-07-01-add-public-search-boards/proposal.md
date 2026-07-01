## Why

Saved searches today are private, cookie-scoped, and addressed by an internal numeric id — a user can replay their own filters but cannot hand anyone a link to them. Sharing a curated job view (e.g. "remote Go jobs in the EU") is a common, low-cost ask that turns a personal convenience into a distribution surface. Every ingredient already exists: the canonical `query` string is a ready-made shareable payload, and the repo already runs a public-read-by-slug pattern for jobs. This change removes exactly two locks from saved searches — owner-only addressing and auth-guarded reads — to make any saved search publicly shareable as a "board".

## What Changes

- A signed-in user can **share** one of their own saved searches, minting a stable public **slug** (readable, derived from the name plus a short random suffix, e.g. `remote-go-jobs-a3f1`).
- Sharing accepts an optional **author label** (free text, e.g. a display handle) stored on the board; empty means the board renders anonymously. `users` is not touched — no profile/display-name subsystem.
- A user can **unshare**, clearing the slug and returning the saved search to private. Unshare intentionally breaks the old link (that is the point); re-sharing mints a fresh slug.
- A new **public, unauthenticated read** returns a shared board by slug: its name, canonical query, and author label — never the owner's id or email.
- The SPA gains a public **`/b/:slug`** route that loads the board and renders its jobs by applying the stored query to the existing jobs list.
- The account area gains a dedicated **"Saved searches" section** (`/my/searches`, reached from the header account menu like the other `/my/*` sections) that lists the user's saved searches and lets them **share/unshare** each one (add the board criterion), copy its public `/b/:slug` link, rename, and delete. Creating a saved search stays in the "My filters" control (that is where the current filters exist); the filters control also keeps a lightweight share/unshare affordance.
- Listing one's own saved searches now also reports each set's shared state (slug + author label) so the UI can show what is shared.

## Capabilities

### New Capabilities
<!-- none; this extends the existing saved-searches capability -->

### Modified Capabilities
- `saved-searches`: adds share/unshare requirements, a nullable public slug and optional author label on the stored set, a public unauthenticated read-by-slug that never leaks owner identity, and the public board UI. Existing private behavior (create/list/update/delete, owner-scoping, name validation, per-user cap) is unchanged.

## Impact

- **DB / migration**: add `saved_searches.public_slug TEXT UNIQUE` (nullable) and `saved_searches.author_label TEXT` (nullable). New migration; sqlc regen.
- **API**: new `POST /api/v1/me/searches/:id/share` and `DELETE /api/v1/me/searches/:id/share` (both `RequireAuth`, cookie-only, like the rest of `/me/searches`); new public `GET /api/v1/boards/:slug` (no auth). The `savedSearchResponse` shape gains `public_slug` and `author_label`.
- **Code**: `internal/savedsearch` service gains Share/Unshare/GetPublicBySlug + slug generation (reuses `internal/normalize`); `internal/handler/me_searches.go` gains the two share handlers; a new public boards handler + route registration in `handler.go`.
- **Web**: new `web/src/routes/b/[slug]` public route; new account section `web/src/routes/my/searches` + a "Saved searches" entry in `HeaderMenu.svelte`; share/unshare UI in both the account section and the filters panel; new share-board API client calls in `web/src/lib/api.ts`; generated TS contract for the board wire shape.
- **Privacy note**: the readable slug is partially guessable by design (chosen for URL friendliness over unguessability); a board is public-by-link only when explicitly shared, and no owner PII is ever exposed.
