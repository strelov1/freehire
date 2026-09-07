## Why

The homepage (`web/src/routes/+page.svelte`) shows only a search box and facet
shortcuts — it moved its job listing to `/jobs` and now carries no signal that
the catalogue is alive. A live "recently added" feed gives first-time visitors
immediate proof that real jobs are flowing in, using data the ingest pipeline
already produces.

## What Changes

- New Postgres table `recent_feed_outbox`, enqueued in the same transaction as
  `UpsertJob` (in `cmd/ingest`'s write path), restricted to canonical,
  non-duplicate, IT postings — the same perimeter `search_outbox` already uses.
- New in-process poller (`internal/job/recentfeed`) running inside the
  long-lived `cmd/server`: drains the outbox every ~10s, groups the batch by
  a normalized role title (`jobhash.NormalizedRoleTitle` — not the existing
  `role_fingerprint` column, which is scoped to one company's own reposts and
  cannot cluster the same role across different companies; see design.md),
  and turns each group into either a single-job event or an aggregated
  "role X seen at N companies" event.
- New in-process broadcaster: a short ring buffer plus fan-out to connected
  SSE clients, so a new connection gets immediate backlog instead of a blank
  feed.
- New public, unauthenticated SSE endpoint `GET /api/v1/feed/recent`
  (`internal/api/handler`), modeled on the existing SSE machinery in
  `internal/api/handler/match_analysis_stream.go`.
- New homepage component `web/src/lib/components/RecentJobsFeed.svelte`:
  subscribes via `EventSource`, renders a short live-updating card list under
  the hero search box. Every card shows a company logo (via the existing
  `EntityLogo` primitive + `companyLogoUrl`, the same resolve-by-name/
  monogram-fallback path `JobRow.svelte` already uses — there is no stored
  `logo_url` to carry over the wire) and the role title; an aggregated card
  is explicit that the sample logo represents one of several companies, not
  all of them.
- `cmd/tg-extract` is deliberately **not** wired into the new outbox (it isn't
  wired into `search_outbox` either today) — out of scope for this change.

## Capabilities

### New Capabilities
- `recent-jobs-feed`: a live, SSE-delivered feed of recently ingested jobs on
  the homepage, grouped by role when many postings of the same role arrive in
  one polling window.

### Modified Capabilities
(none — this introduces a new capability and does not change the documented
behavior of `source-ingest`, `job-search`, or `web-frontend`; the new outbox
enqueue is an internal implementation detail of the ingest write path, not a
change to any of their existing requirements)

## Impact

- **DB**: new migration adding `recent_feed_outbox`; new sqlc queries
  (enqueue, claim-and-delete a batch).
- **`cmd/ingest`**: `store.go`'s `persist`/`save` gains one more conditional
  enqueue call next to `EnqueueSearchOutbox`.
- **`cmd/server`**: starts the new poller goroutine at boot and wires the
  broadcaster into the new handler.
- **`internal/api/handler`**: one new SSE route.
- **`internal/job/recentfeed`** (new package): must be added to
  `internal/platform/arch/layering/blocks.go` under the `job` block.
- **Frontend**: one new component, wired into the homepage route only.
- No breaking changes; no changes to existing public API responses.
