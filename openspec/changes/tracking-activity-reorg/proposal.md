## Why

The `/my/tracking` section conflates two unrelated concerns: the active application
process (Board + Pipeline) and the things a user looks back at or is still weighing
(Saved wishlist, view History, AI-fit Matches). The kanban's **Saved** column in
particular sits inside the active pipeline it does not belong to. Splitting the
section along this seam makes each area's purpose obvious.

## What Changes

- **Tracking** (`/my/tracking`) keeps only the **Board** and **Pipeline** sub-tabs.
- The kanban **Board loses its Saved column**; columns become `Applied · Interview · Offer · Closed`.
- **BREAKING (UI navigation):** History and Matches (AI fit) move out of Tracking. The old `/my/tracking/history` and `/my/tracking/analyses` URLs are retired **without** redirects (they are `noindex`, auth-gated personal pages with no external links).
- New **Activity** sidebar section (`/my/activity`) with three internal tabs: **Saved · History · Matches** (Saved is the index tab).
- **Saved** is a new list backed by the existing `listMyJobs('saved')` filter — no new data or backend behavior.
- Board drawer behavior: choosing **"No stage"** now takes a card **off the board** (it becomes saved-only and appears under Activity → Saved) instead of demoting it to a Saved column.

Frontend-only (`web/` SvelteKit). No backend, sqlc, DB, or API change — the `saved`
list filter and the `clearJobStage` "keep the saved mark" semantics already exist.

## Capabilities

### New Capabilities
<!-- none — reusing existing account-navigation + user-job-tracking capabilities -->

### Modified Capabilities
- `account-navigation`: the section-navigation item list gains an **Activity** entry; the "Tracking sub-navigation preserved" requirement is reduced to Board + Pipeline (History/Matches no longer sub-tabs of Tracking).
- `user-job-tracking`: a new requirement for the **Activity** section (Saved/History/Matches tabs) and its Saved list; the frontend Tracking-section requirement is updated to Board + Pipeline only; the board's Saved-column removal and the "No stage removes from board" behavior are specified.

## Impact

- Affected code (all under `web/`): `lib/accountNav.ts`, `routes/my/+layout.svelte`, `routes/my/tracking/+layout.svelte`, `lib/board.ts`, `lib/components/JobBoard.svelte`; new `routes/my/activity/*` and `lib/components/SavedJobs.svelte`; `routes/my/tracking/history` and `.../analyses` removed (moved under `activity/`).
- No backend/API/DB/sqlc changes. `JobHistory.svelte` and `AnalysesView.svelte` are reused unchanged; only their route wrappers move.
- Verification via `svelte-check` + `eslint` + `vitest` locally (web CI does not gate these) plus manual/visual check of the board and the Activity tabs.
