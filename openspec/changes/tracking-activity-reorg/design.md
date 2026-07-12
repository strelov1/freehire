## Context

The `my/*` account section uses a two-level navigation: `lib/accountNav.ts` is a
pure, unit-testable model of the top-level sidebar/strip, and each section that
needs sub-views (currently only Tracking) owns its own tab layout with one URL per
tab. Tracking today carries four sub-tabs — Board, Pipeline, History, Matches — mixing
the active application pipeline with read-back views. The kanban column is a *view*
over `saved_at`/`applied_at`/`stage` (`lib/board.ts::columnOf`), not a stored field.

The full narrative design lives at
`docs/superpowers/specs/2026-07-12-tracking-activity-reorg-design.md`; this document
records the decisions that matter for implementation.

## Goals / Non-Goals

**Goals:**
- Tracking = active pipeline only (Board + Pipeline); Board without a Saved column.
- New Activity section (`/my/activity`) with Saved · History · Matches tabs.
- Reuse existing components and backend behavior; frontend-only change.

**Non-Goals:**
- No backend, API, DB, or sqlc change (the `saved` filter and `clearJobStage`
  keep-saved-mark semantics already exist).
- No redirects from the retired `/my/tracking/history` and `/my/tracking/analyses`.
- No change to the internals of Pipeline, History, or Matches views.

## Decisions

**Activity is a new top-level section, not a fourth Tracking tab.** The two areas
answer different questions ("what am I working on" vs "what have I seen / saved / been
matched to"). Alternative — keep everything under Tracking with more tabs — was rejected
as the status quo this change exists to undo.

**Saved lives in Activity, as the index tab.** It is the most actionable of the three
(a wishlist to act on), so it is the default view of the section. Alternative — its own
sidebar item — was rejected to keep the sidebar short; alternative — a Tracking tab —
contradicts the goal of removing Saved from the active pipeline.

**`columnOf` returns `BoardColumnId | null`.** `null` marks a saved-only row (the old
wishlist fallback) that no longer has a board column. `JobBoard.load()` fetches the
`board` set (saved ∪ applied ∪ stage, capped at 500) and drops `null` rows client-side.
Alternative — a new backend `board`-without-saved filter — is unnecessary for a small,
already-fetched set and would add API surface for no benefit.

**"No stage" removes the card from the Board.** With no Saved column to demote into,
clearing a stage makes the job saved-only. The drawer path mirrors the existing
`remove()` (filter the card out, drop `cardCol[id]`, close) but calls
`api.saveJob(id)` + `api.clearJobStage(id)` (keeps the saved mark) instead of
`api.untrackJob(id)`. The job reappears under Activity → Saved.

**`SavedJobs.svelte` is a thin twin of `JobHistory.svelte`.** Same
Paginator + JobRow + LoadMore + States shape, differing only in the filter
(`listMyJobs('saved')`) and the empty message. A shared parametrized list component was
considered but rejected as premature — two small twins are clearer than one abstraction
with a mode flag; revisit if a third such list appears.

## Risks / Trade-offs

- [Stale bookmarks to `/my/tracking/history` or `.../analyses` 404] → Accepted:
  `noindex`, auth-gated, no external links. Add redirects later only if telemetry shows
  hits (noted seam).
- [`board` fetch returns saved-only rows the board then discards] → Mostly negligible,
  but note the cap: saved-only rows count toward the 500-row fetch, and the server orders
  by most-recent activity, so a user with 500+ tracked jobs and many recent saves could
  have older active applications fall outside the window and vanish from the board.
  Accepted at current scale and documented in `JobBoard.load()`; the clean fix is a
  server-side board-minus-saved filter, deferred until it bites.
- [`columnOf` signature change to `| null` touches every caller] → Callers are
  `JobBoard.svelte` only (plus the unit test); the compiler flags each site.

## Migration Plan

Single frontend deploy; no data migration, no backend deploy, no env change. Rollback is
a revert of the frontend commit. No feature flag needed.
