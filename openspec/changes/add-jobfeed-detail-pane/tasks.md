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
- [x] 4.2 Verified structurally + via scripted browser check: the pane is driven by `page.params.slug` and `/jobs/[slug]`'s own load doesn't read `url.searchParams`, so a filter-driven query change never touches the open pane — matches decision 1.3 for free

## 5. Detail pane behavior

- [x] 5.1 N/A as implemented: `/jobs/[slug]`'s `JobView` was reused unchanged and only ever renders the lightweight deterministic `JobMatch`, not the LLM `MatchAnalysisFull` report — confirmed by screenshot ("Profile Match" card, not the AI verdict/gauge). No forced-recompute risk exists on this page today.
- [x] 5.2 Verified via scripted browser scroll: InfiniteScroll's sentinel correctly triggers inside the new `overflow-y-auto` rail column (20 -> 80 cards after scrolling the inner container, not the window)

## 6. Mobile fallback

- [x] 6.1 Verified via screenshot at 390px: `/` shows list only
- [x] 6.2 Verified via screenshot at 390px: `/jobs/[slug]` shows detail only, list hidden — same URL desktop and mobile both use

## 7. Verification

- [x] 7.1 Verified via scripted browser (playwright-core + system Chrome, against prod's read API): select A -> B -> Back restores job A's URL and title exactly
- [x] 7.2 Verified: child page load doesn't read `url.searchParams` (only `params.slug`), so a filter-driven query change on the layout never reruns/refetches the open job's own data — see 4.2
- [x] 7.3 Verified via screenshot: direct navigation to a job URL server-renders both the detail pane and the list column together
- [x] 7.4 Verified via screenshot at 390px: `/` shows list only, `/jobs/[slug]` shows detail only (same URLs desktop and mobile share)
- [x] 7.5 Verified structurally (see 4.2/7.2) — a filter's query-string change never touches the child route's own load, so an open pane is never disturbed
- [x] 7.6 `go vet -tags=integration ./...` clean; `npm run lint` 0 errors (pre-existing warnings only); `npm run check` 25 errors/18 warnings — identical count and files to the pre-change baseline, none new
