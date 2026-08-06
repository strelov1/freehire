## 1. Resolve open design questions

- [x] 1.1 Decided: `fit`/`discussion` stay full-page, outside the shared layout (no list column) — out of scope for this change
- [x] 1.2 Decided: `/` with nothing selected shows an empty "pick a job" placeholder (matches the `/tailor` precedent), no auto-select
- [x] 1.3 Decided: an already-open detail pane stays open when a filter change drops the selected job from the list

## 2. Route restructuring

- [x] 2.1 Moved `web/src/routes/+page.svelte`/`+page.server.ts` and `web/src/routes/jobs/[slug]/` under `web/src/routes/(feed)/`; verified `/`, `/jobs/[slug]`, `/discussion`, `/copies`, `/fit` all still resolve (svelte-check clean, dev-server smoke check)
- [x] 2.2 Created `(feed)/+layout.server.ts` (list fetch, no redirect) and `(feed)/+layout.svelte` (responsive two-column shell: list column + `{@render children()}` pane, `lg:` breakpoint toggles which shows on mobile)
- [x] 2.3 `(feed)/+page.server.ts` now only owns the first-visit redirect (list fetch moved to the layout, no double-fetch); `(feed)/+page.svelte` renders the "pick a job" placeholder
- [x] 2.4 `fit`/`copies`/`discussion` reset past the `(feed)` layout via `+layout@.svelte` (same convention as `my/assistant/+layout@.svelte`); `og.png` is a `+server.ts` endpoint, unaffected by layouts either way

## 3. List column

- [x] 3.1 List column passes `compact={layout === 'stacked'}` to `JobRow` — verified via screenshot against live prod data (blurb hidden, tighter padding)
- [x] 3.2 List column fixed to `lg:w-[440px]` in `(feed)/+layout.svelte`
- [x] 3.3 `JobRow` gained a `selected` prop (brand border + ring); layout derives it from `page.params.slug` — verified via screenshot

## 4. Filter bar relocation

- [x] 4.1 Added a `layout: 'sidebar' | 'stacked'` prop to `JobsView`; `stacked` stacks the unchanged `FilterSummary` card above the list in one column (not a literal horizontal chip bar — FilterSummaryShell is a vertical block by design; `sidebar` mode, used by collections/company pages, is pixel-identical to before)
- [ ] 4.2 Wire 1.3's decision: clearing or preserving the open pane when the selected job drops out of the filtered list — NOT YET DONE, needs its own verification pass

## 5. Detail pane behavior

- [x] 5.1 N/A as implemented: `/jobs/[slug]`'s `JobView` was reused unchanged and only ever renders the lightweight deterministic `JobMatch`, not the LLM `MatchAnalysisFull` report — confirmed by screenshot ("Profile Match" card, not the AI verdict/gauge). No forced-recompute risk exists on this page today.
- [x] 5.2 CSS is in place (`lg:sticky ... lg:overflow-y-auto` on the list column) — structurally verified, NOT interaction-tested (only static screenshots taken so far)

## 6. Mobile fallback

- [x] 6.1 Verified via screenshot at 390px: `/` shows list only
- [x] 6.2 Verified via screenshot at 390px: `/jobs/[slug]` shows detail only, list hidden — same URL desktop and mobile both use

## 7. Verification

- [ ] 7.1 Manual check: selecting jobs A → B → Back restores job A's detail and matching URL (browser back/forward, not a client-side URL rewrite) — the exact regression class in [[hire-shallow-routing-back-forward-stale]]
- [ ] 7.2 Manual check: the shared layout's list `load` does not re-run (list does not re-fetch/flash) when only the child job page navigates
- [ ] 7.3 Manual check: opening a `/jobs/[slug]` URL fresh (no prior client-side nav) server-renders the detail pane populated, with the list alongside, no client-loading flash for primary content
- [ ] 7.4 Manual check at a narrow (<lg) viewport: list-only layout, card selection navigates full-page to `/jobs/[slug]`
- [ ] 7.5 Manual check: applying a filter updates the list and URL without disturbing an open, still-matching detail pane
- [ ] 7.6 `go vet -tags=integration ./...` and existing web checks (lint/check) pass before PR, per repo convention
