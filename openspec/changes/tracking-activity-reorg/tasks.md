## 1. Board: remove the Saved column

- [x] 1.1 Update `web/src/lib/board.ts`: drop `'saved'` from `BoardColumnId` and `BOARD_COLUMNS`; change `columnOf` to return `BoardColumnId | null` (null = saved-only). Add/extend a `board.test.ts` covering: stage → column, applied-without-stage → `applied`, saved-only → `null`.
- [x] 1.2 Update `web/src/lib/components/JobBoard.svelte`: drop `saved` from `emptyColumns()`; in `load()` skip rows whose `columnOf` is `null`; remove the `case 'saved'` in `persistMove`; make the drawer "No stage" path remove the card from the board (filter it out of its column, drop `cardCol[id]`, close drawer) while calling `api.saveJob` + `api.clearJobStage` (keep the saved mark).

## 2. Activity section

- [x] 2.1 Add `web/src/lib/components/SavedJobs.svelte` — a twin of `JobHistory.svelte` using `listMyJobs('saved', …)` and a saved-specific empty message.
- [x] 2.2 Add `web/src/routes/my/activity/+layout.svelte` — tab strip `Saved · History · Matches` (mirror `tracking/+layout.svelte`), `<h1>Activity</h1>`, base `<title>`.
- [x] 2.3 Add `web/src/routes/my/activity/+page.svelte` (index → `SavedJobs`), `web/src/routes/my/activity/history/+page.svelte` (→ `JobHistory`), `web/src/routes/my/activity/matches/+page.svelte` (→ `AnalysesView`). Set each page `<title>`.
- [x] 2.4 Remove `web/src/routes/my/tracking/history/` and `web/src/routes/my/tracking/analyses/` (moved under `activity/`).

## 3. Tracking sub-navigation reduced

- [x] 3.1 Update `web/src/routes/my/tracking/+layout.svelte`: keep only Board and Pipeline tabs; remove the History and Matches tabs and their `historyActive`/`analysesActive` deriveds.

## 4. Sidebar: add Activity

- [x] 4.1 Update `web/src/lib/accountNav.ts`: insert `{ href: '/my/activity', label: 'Activity' }` after Tracking. Update `web/src/lib/accountNav.test.ts` to assert the new item and its ordering / active-matching.
- [x] 4.2 Update `web/src/routes/my/+layout.svelte`: add the `'/my/activity'` key to the `icons` map with a Lucide icon (e.g. `History` or `Sparkles`).

## 5. Apply confirmation links to the board

- [x] 5.1 Update `web/src/lib/components/JobView.svelte`: after `confirmApplied()` succeeds, surface a confirmation with a link to `/my/tracking` (a `justApplied` state set on success and reset when the job changes, beside the existing `showApplyPrompt` reset). "No" shows no link.

## 6. Verify

- [x] 6.1 Run `svelte-check`, `eslint`, and `vitest` in `web/`; all green.
- [ ] 6.2 Manual/visual check (needs authed running stack — NOT yet done): Board columns Applied·Interview·Offer·Closed; drag between stages and to Closed (outcome drawer); "No stage" removes a card and it appears under Activity → Saved; Activity tabs Saved/History/Matches load and are reload-safe; sidebar marks Activity active on all three; confirming "Did you apply? → Yes" on a job page shows a link to `/my/tracking`.
