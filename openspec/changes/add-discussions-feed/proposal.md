## Why

Discussions are reachable only from a subject you are already looking at. A
thread lives at `/jobs/<slug>/discussion` or `/companies/<slug>/discussion`, so
the only way to find one is to guess which of 8M postings someone happened to
write about. There is no page that answers "what is being discussed here at
all", which means a reader cannot discover the surface and an author writes into
a room nobody can find. Six weeks after launch the catalogue holds 3 threads and
0 replies; the primitive works, its only entrance is a door you must already be
standing at.

The same read path also mis-reports its own pagination: the list emits a
`next_cursor` whenever it returned ANY row, so a single-thread subject renders a
"Load more" button that fetches an empty page. Both defects live in the same
listing code, so they ship together.

## What Changes

- Add a **global discussions feed** at `/discussions`: every open thread across
  every subject, newest first, keyset-paged — the section that makes the surface
  discoverable.
- Each row names its subject in **human terms** (a vacancy's title and company,
  a company's name), not the stored slug. The subject reference carries no FK,
  so a row whose subject was pruned falls back to the slug rather than
  disappearing.
- **Fix the pagination cursor** on both thread and reply listings: emit
  `next_cursor` only when the page returned is full, so "Load more" appears only
  when there is a next page.
- The feed is read-only. Starting a topic still requires a subject and stays on
  the subject's own page.

## Capabilities

### New Capabilities
<!-- none: this extends the existing community-threads capability -->

### Modified Capabilities
- `community-threads`: gains a cross-subject listing of open threads that
  resolves each thread's subject to its display name, and a corrected
  end-of-pagination signal on the existing listings.

## Impact

- **New migration**: a partial index on `threads (created_at DESC, id DESC)
  WHERE status = 'open'`. The existing index leads with the subject, so it
  cannot serve an unfiltered newest-first scan.
- **New sqlc queries** in `internal/platform/db/queries/community.sql` — the
  unfiltered keyset pair, each LEFT JOINing `jobs` and `companies` to resolve
  the subject's display name.
- **`internal/engage/community`**: a `ThreadWithSubject` read model, a
  `ListRecentThreads` use case, and `PageSize()` exposed so the handler can tell
  a full page from a last page.
- **`internal/api/handler/community.go`**: `GET /api/v1/threads/recent`
  (public), registered before `/threads/:id`; the cursor fix in `ListThreads`
  and `GetThread`.
- **Frontend** (`web/`): a `/discussions` route, a `DiscussionFeed` component, a
  footer link, and the page added to `sitemap-pages.xml`.
- Reuses the existing thread-detail routes: a feed row links to the subject's
  own thread page, so no new thread-reading surface is added.
- **Not in this change**: `web/static/openapi.yaml` documents no discussion
  endpoint at all, so documenting only the new one would leave a lopsided
  contract. Recorded as a debt, not addressed here.
