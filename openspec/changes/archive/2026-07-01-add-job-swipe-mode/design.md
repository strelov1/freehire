## Context

The catalogue is served from Meilisearch (`internal/search`, index `jobs`,
primary key `id`) via `GET /api/v1/jobs/search`; per-user interactions live in
Postgres `user_jobs` (`viewed_at`/`applied_at`/`saved_at`, composite PK
`(user_id, job_id)`) and are written by slug-addressed endpoints behind
`RequireAuthOrKey` (`internal/handler/user_jobs.go`, delegating to the
`jobtracking` service). The `/jobs` page (`web/src/lib/components/JobsView.svelte`)
builds its query with `FilterStore` + `filtersToParams` and reads it back from
the URL, so filters and free-text `q` round-trip through the URL.

The reference is the Tinder swipe pattern: a full-screen card deck where a single
gesture decides each item. We add it as a triage layer over the existing search

- tracking machinery — no new catalogue wire shape, reusing `jobview` and the
save interaction.

## Goals / Non-Goals

**Goals:**

- One swipe deck that works on mobile (touch gestures) and desktop
  (buttons + arrow keys), reachable from `/jobs` with the current filters.
- Right = save (existing interaction), left = dismiss (new mark); the deck shows
  only jobs the user has not already judged (saved or dismissed).
- The deck matches the list's ranking/filtering exactly by reusing the same
  Meilisearch query.
- One-step undo for the last save/dismiss.

**Non-Goals:**

- Dismissal does NOT hide a job from the `/jobs` list or search — it only affects
  the swipe deck.
- No new recommendation/ranking logic; the deck order is search order.
- No third gesture for "apply" in v1 (apply stays on the detail page).
- No cross-storage per-user filtering of the main catalogue.

## Decisions

### Dismiss as a new `user_jobs` column, mirroring save

Add `dismissed_at TIMESTAMPTZ` to `user_jobs` (migration mirrors the existing
`saved_at ALTER`). New sqlc queries `DismissJob` (idempotent upsert of
`dismissed_at = now()`) and `UndismissJob` (set `dismissed_at = NULL`) mirror
`SaveJob`/`UnsaveJob` exactly, and the handlers `POST|DELETE /jobs/:slug/dismiss`
(behind `RequireAuthOrKey`) mirror `SaveJob`/`UnsaveJob`. This keeps the new
interaction inside the established pattern rather than inventing a parallel one.

### Deck endpoint = search + server-side `id NOT IN` exclusion

`GET /api/v1/me/jobs/swipe` (behind `RequireAuth` — needs a user) reuses the
`SearchJobs` handler's param parsing and the same `search` call, adding one
filter group: `NotIn("id", excluded)`, where `excluded` is
`ExcludedJobIDs(user)` — the caller's job ids with `dismissed_at IS NOT NULL OR
saved_at IS NOT NULL`. A new `NotIn(attr, ids)` helper in `internal/search/filter.go`
emits `id NOT IN [1,2,3]`, escaped and composed through the existing `Filter`
AND/OR grouping. The excluded list is capped (e.g. 1000 most-recent) so the
filter stays bounded; a heavy triager who exceeds the cap may occasionally see a
long-ago-judged job, which is acceptable.

Rejected alternative — excluding on the client after `searchJobs` — leaks the
exclusion set to the browser and creates pagination holes; rejected alternative —
querying Postgres `jobs` directly — would reimplement Meilisearch's full-text and
facet filtering and drift from list ranking.

### `id` becomes a Meilisearch filterable attribute

The exclusion filter requires `id` in `FilterableAttributes` (it is currently
only the primary key). Adding it triggers a one-time reindex on deploy; per the
known Meilisearch behavior, a newly-declared filterable attribute makes `/jobs`
return 500 until the index is rebuilt, so the deploy MUST reindex first.

### Frontend: dedicated route + one deck component

A new `/jobs/swipe` route (`+page.svelte`, plus a minimal `+page.server.ts` for
`noindex`) reads the same URL params as `/jobs`. A toolbar button in
`JobsView.svelte` navigates there via `goto('/jobs/swipe?' + filtersToParams(...))`.
`SwipeDeck.svelte` owns the deck: it fetches via a new `api.swipeDeck(params,
limit, offset)`, renders the active card plus a peeking next card, and handles
gestures/buttons/keys. Save/dismiss call `api.saveJob` / new `api.dismissJob`;
undo calls `api.unsaveJob` / new `api.undismissJob` and restores the popped card.
Prefetch fetches the next `offset` batch when remaining cards fall below a
threshold; because the server excludes saved/dismissed at query time, batches do
not repeat judged jobs. Signed-out users get a sign-in prompt instead of a deck.

The card uses the richer layout (logo, title, company + location, salary, facet
chips, short description excerpt) — all fields already on `jobview`.

## Risks / Trade-offs

- **Reindex-on-deploy window** — the new filterable `id` 500s `/jobs` until the
  index rebuilds. Mitigation: reindex as the first deploy step (documented in the
  tasks), consistent with prior new-filterable-attribute changes.
- **Excluded-id list growth** — a user who dismisses thousands of jobs makes the
  `NOT IN` list large. Mitigation: cap the list to the most-recent N; the
  overflow risk is only an occasional re-shown old job, never a correctness bug.
- **No web unit-test runner** — `web/` has no test runner; frontend tasks are
  verified with `svelte-check` + lint and a manual/headless pass, matching repo
  convention. Backend tasks use Go tests (queue/handler integration under the
  `integration` build tag where DB access is needed).
- **Undo scope** — only the last action is reversible; a mis-swipe two cards back
  is corrected via the normal saved list / by re-encountering the job after
  `UndismissJob`. Accepted to keep the deck state simple.

## Migration Plan

1. Apply the `user_jobs.dismissed_at` migration (manual psql on prod per the
   repo's migration convention).
2. Deploy backend with `id` added to filterable attributes, then run `reindex`
   **before** serving — or reindex first if deploy order allows — to avoid the
   `/jobs` 500 window.
3. Deploy `web/`. Rollback is reverting the web + backend commits; the column and
   reindex are additive and safe to leave in place.
