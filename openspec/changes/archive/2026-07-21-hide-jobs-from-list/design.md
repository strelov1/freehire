## Context

Per-user job interactions live in `user_jobs`, one row per `(user, job)`. The
swipe deck already introduced a **dismiss** mark (`dismissed_at`) with idempotent
`DismissJob`/`UndismissJob` queries, `POST/DELETE /api/v1/jobs/:slug/dismiss`
endpoints (integration-tested), and `api.dismissJob`/`api.undismissJob` clients.
Today dismiss is surfaced only in `SwipeDeck`; the browse feed neither offers it
nor excludes dismissed jobs.

The browse feed (`JobsView` → `JobRow`) already dims viewed cards and fills saved
bookmarks by cross-referencing client-side slug sets loaded once for signed-in
users: `viewedJobs.svelte.ts` (from `GET /me/tracking/viewed`) and
`savedJobs.svelte.ts` (from `GET /me/tracking/saved`). These read the same
`user_jobs` marks the tracking service already exposes. This change adds a third
such set — dismissed — and a hide gesture that writes it.

The Activity surface (`my/activity/+layout.svelte`) is a tabbed set of routed
views (Saved / History / Matches), each backed by `api.listMyJobs(filter, …)`
over `ListUserJobs`, whose `filter` is a controlled vocabulary
(`all|viewed|saved|applied|board`).

## Goals / Non-Goals

**Goals:**

- Bring the existing dismiss gesture to the browse feed with a discoverable,
  non-navigating hide control and an undo path.
- Keep hidden jobs out of the feed using the established client-side slug
  cross-reference pattern — no new server query shape for the feed itself.
- Give users a place (Activity → Hidden) to review and reverse hides.

**Non-Goals:**

- Server-side / Meili exclusion of dismissed jobs from search. Per-user state is
  not in the index; the client cross-reference is sufficient for the expected
  small dismissed set, matching how viewed/saved already work.
- A general-purpose global toast system. The undo affordance is a single local
  component owned by the feed, not shared infra.
- Changing the dismiss endpoint contract or the swipe deck.

## Decisions

### Decision: Reuse the `dismissed_at` mark, do not invent a "hidden" mark

Hiding a job from the feed and swiping it away are the same user intent — "stop
showing me this." Both map to `dismissed_at`. **Alternative considered:** a
separate `hidden_at` column to distinguish feed-hide from swipe-dismiss. Rejected
as YAGNI: no requirement distinguishes them, and a second column would fork the
exclusion logic, the endpoints, and the undo path for no user-visible gain.

### Decision: Client-side feed exclusion via a `dismissedJobs.svelte.ts` store

Add a store that is a near-exact copy of `savedJobs.svelte.ts` (a `SvelteSet` of
slugs behind `UserResource`), loaded from a new `GET /me/tracking/dismissed`. The
feed filters `jobs.items` by this set. **Alternative:** filter server-side.
Rejected — the feed is Meili-backed and per-user dismiss state is not indexable
without fetching the dismissed id set and injecting a `NOT IN` filter per query,
far heavier than the client cross-reference the codebase already uses for
viewed/saved. Trade-off: a page of N results may render fewer than N cards after
exclusion; acceptable for the expected small dismissed set (same trade-off the
viewed-dim path already lives with).

### Decision: Hide control mirrors the save control on `JobRow`

An icon-only overlay button (`EyeOff`), sibling of the card `<a>` (not a
descendant) so it never navigates, revealed on hover. Optimistic: add to the
dismissed set + fire `api.dismissJob`, roll back on failure; signed-out click
opens the auth dialog. This is the exact shape of the existing `toggleSave`.
Placement: bottom corner of the card (the top-right corner is taken by the
bookmark), satisfying "somewhere at the bottom, on hover."

### Decision: Local undo toast owned by `JobsView`

On hide, `JobsView` shows a single fixed-position "Job hidden — Undo" element with
a timeout. Undo calls `api.undismissJob` and removes the slug from the dismissed
set (restoring the card). **Alternative:** in-place collapse-to-strip, or a global
toast system. Rejected the global system as premature infra; rejected in-place
collapse because the user chose vanish-with-toast. The component is self-contained
and leaves a clean seam if a shared toast is ever needed.

### Decision: `dismissed` filter on the tracking list, not a new endpoint

The Hidden tab reuses `api.listMyJobs` with a new `dismissed` filter value, added
to `ListUserJobs`/`CountUserJobs` (`WHERE … dismissed_at IS NOT NULL`) and the
`MyJobsFilter` vocabulary. This matches Saved/History, which are also filter
values over the same list. The dismissed-slugs cross-reference is the one new
read endpoint, mirroring `ListSavedSlugs` exactly.

## Risks / Trade-offs

- **Short feed pages after exclusion** → Acceptable and pre-existing; the
  paginator keeps loading more, and the dismissed set is small per user.
- **Optimistic hide races a slow dismiss request** → The `saving`-style guard and
  rollback-on-failure from `toggleSave` are reused, so a failed dismiss restores
  the card.
- **Undo after the toast times out** → Not supported inline by design; the
  Activity → Hidden tab is the durable recovery path, so nothing is unrecoverable.
- **Two surfaces write the same mark (swipe + feed)** → Idempotent upserts already
  guarantee one row per `(user, job)`; concurrent surfaces converge safely.

## Migration Plan

No schema migration: `dismissed_at`, the queries, and the endpoints already
exist. New SQL (`ListDismissedJobSlugs`, `dismissed` filter branch) regenerates
via sqlc. Rollback is a pure code revert — no data shape changes, and any
`dismissed_at` values already written by the swipe deck remain valid.
