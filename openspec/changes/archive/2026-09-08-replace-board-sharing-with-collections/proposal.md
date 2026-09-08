## Why

Public saved-search sharing (`/b/:slug` "boards") lets a user publish a live, unauthenticated re-run of a search query. It sees little use and conflates two different things under one slug: "the filters I like" and "the specific jobs I want someone else to see" — the latter is what people actually want to share (a hand-picked shortlist), and a query-replay drifts as the catalogue changes underneath it. We are retiring board sharing and replacing it with **job lists**: personal, named lists of specific jobs (independent of the existing per-job "save" star) that a user can optionally publish read-only by slug, the way boards worked, but pointing at a fixed set of jobs instead of a live query.

## What Changes

- **BREAKING**: Remove public saved-search sharing entirely — `POST/DELETE /api/v1/me/searches/:id/share`, the public `GET /api/v1/boards/:slug` endpoint, and the `/b/:slug` web route. Existing public board links stop resolving (404) with no redirect and no migration path into job lists.
- Remove `public_slug` and `author_label` from `saved_searches` (columns, unique index, and every code path that reads or writes them).
- Add **job lists**: a signed-in user can create named lists, add/remove specific jobs to/from a list (a job may belong to any number of lists, independent of the existing `save` star), rename or delete a list, and optionally publish one read-only by minting a public slug (mirroring the board slug scheme: transliterated name + random suffix, retried on collision).
- Add a public, unauthenticated route that renders a shared list's name, description, and its jobs (closed/expired jobs stay listed, marked as such — a list is the user's own record of what they looked at, not a live availability feed).
- Add an account-area management page for job lists (create/rename/delete, add/remove jobs, share/unshare, copy the public link), and an "Add to list" affordance on the job card alongside the existing "Save" star.

## Capabilities

### New Capabilities
- `saved-job-lists`: signed-in users create named lists of specific jobs (distinct from the single-flag "save"), manage list membership, and optionally publish a list read-only via a public slug.

### Modified Capabilities
- `saved-searches`: remove the "Share a saved search as a public board", "Unshare a public board", "Public read of a shared board by slug", and "Public board page in the web app" requirements; update "List saved searches" to drop the `public_slug`/`author_label` fields from the response; update "Saved searches section in the account area" to drop the share/unshare/copy-link actions from that page.

## Impact

- **DB**: new tables `job_lists`, `job_list_items`; migration dropping `saved_searches.public_slug`, `saved_searches.author_label`, and their unique index.
- **Backend**: new package `internal/application/joblists` (layer 6, sibling to `jobtracking`); the slug-minting helper moves out of `internal/search/savedsearch/share.go` into a shared platform-layer package (its only other consumer is being deleted, so this is a move, not new shared infrastructure); `internal/search/savedsearch` loses `Share`/`Unshare`/`GetPublicBoard`/`share.go`/the `Board` type; `internal/api/handler` loses the `GET /boards/:slug` handler and the share/unshare routes in `me_searches.go`, and gains the job-lists routes; sqlc queries for saved-search sharing are deleted, new ones added for job lists.
- **Frontend**: `web/src/routes/b/[slug]` is deleted; a new public route renders a shared job list; `SavedSearchesView.svelte` loses its share/unshare/copy-link UI; a new account-area page and job-card affordance are added for job lists; `api.ts` and `types.ts` are updated accordingly.
