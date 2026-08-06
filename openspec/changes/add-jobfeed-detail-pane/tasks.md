## 1. Resolve open design questions

- [x] 1.1 Decided: `fit`/`discussion` stay full-page, outside the shared layout (no list column) — out of scope for this change
- [x] 1.2 Decided: `/` with nothing selected shows an empty "pick a job" placeholder (matches the `/tailor` precedent), no auto-select
- [x] 1.3 Decided: an already-open detail pane stays open when a filter change drops the selected job from the list

## 2. Route restructuring

- [x] 2.1 Moved `web/src/routes/+page.svelte`/`+page.server.ts` and `web/src/routes/jobs/[slug]/` under `web/src/routes/(feed)/`; verified `/`, `/jobs/[slug]`, `/discussion`, `/copies`, `/fit` all still resolve (svelte-check clean, dev-server smoke check)
- [ ] 2.2 Create `(feed)/+layout.svelte` + `+layout.server.ts`/`+layout.ts`: owns the job list fetch, filter state, and renders the list + filter bar
- [ ] 2.3 Trim `(feed)/+page.svelte`/`+page.server.ts` to just the no-selection state decided in 1.2 — it must not also fetch the list (avoid double-fetch)
- [x] 2.4 `fit`/`copies`/`discussion` reset past the `(feed)` layout via `+layout@.svelte` (same convention as `my/assistant/+layout@.svelte`); `og.png` is a `+server.ts` endpoint, unaffected by layouts either way

## 3. List column

- [ ] 3.1 Switch the list column's `JobRow` cards to the existing `compact` presentation
- [ ] 3.2 Fix the list column to a ~420–460px width (validated by the design spike) inside the shared layout
- [ ] 3.3 Add selected-card visual state (border/ring) driven by the current route's `slug` param, not client-side click state

## 4. Filter bar relocation

- [ ] 4.1 Move `JobsView`'s filter controls from the `<aside>` sidebar into a horizontal bar docked above the list column; keep `FilterStore`/URL-sync logic unchanged
- [ ] 4.2 Wire 1.3's decision: clearing or preserving the open pane when the selected job drops out of the filtered list

## 5. Detail pane behavior

- [ ] 5.1 Seed `MatchAnalysisFull`/`JobMatch` from the job's cached `initial` fit with `autoRun={false}` (mirror `ArtifactPanel.svelte`'s usage) — no forced recompute per card click
- [ ] 5.2 Verify the list column and detail pane scroll independently, following the `/tailor/[slug]` two-column shell pattern

## 6. Mobile fallback

- [ ] 6.1 Below the desktop breakpoint, render the list alone (no docked pane) — reuse the breakpoint convention `ArtifactPanel`/`tailor/[slug]` already use
- [ ] 6.2 Confirm card links still point at `/jobs/[slug]` (unchanged) and that a tap there navigates full-page on mobile, exactly as today

## 7. Verification

- [ ] 7.1 Manual check: selecting jobs A → B → Back restores job A's detail and matching URL (browser back/forward, not a client-side URL rewrite) — the exact regression class in [[hire-shallow-routing-back-forward-stale]]
- [ ] 7.2 Manual check: the shared layout's list `load` does not re-run (list does not re-fetch/flash) when only the child job page navigates
- [ ] 7.3 Manual check: opening a `/jobs/[slug]` URL fresh (no prior client-side nav) server-renders the detail pane populated, with the list alongside, no client-loading flash for primary content
- [ ] 7.4 Manual check at a narrow (<lg) viewport: list-only layout, card selection navigates full-page to `/jobs/[slug]`
- [ ] 7.5 Manual check: applying a filter updates the list and URL without disturbing an open, still-matching detail pane
- [ ] 7.6 `go vet -tags=integration ./...` and existing web checks (lint/check) pass before PR, per repo convention
