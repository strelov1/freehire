## 1. Navigation model (pure, unit-tested)

- [x] 1.1 Add `web/src/lib/accountNav.ts` exporting the six-item nav config
  (`{ href, label }`) and a pure `isSectionActive(path, href)` helper
  (`path === href || path.startsWith(href + '/')`). Icons are mapped in the
  layout, keeping this module Svelte-free and unit-testable.
- [x] 1.2 Add `web/src/lib/accountNav.test.ts` (vitest): exact match active;
  descendant path active (`/my/tracking/pipeline` → Tracking); non-matching
  sibling not active; segment-boundary guard; config shape (6 items, `/my/`
  hrefs, unique).

## 2. Shell layout

- [x] 2.1 Create `web/src/routes/my/+layout.svelte`: `mx-auto w-full max-w-6xl
  px-4 py-6` container, `noindex` in `<svelte:head>`, unified auth-gate
  (signed-out → single sign-in prompt + `openAuthDialog` action), signed-in →
  vertical sidebar (`hidden lg:block w-56 shrink-0`, sticky) + horizontal strip
  (`lg:hidden overflow-x-auto`) driven by `accountNav`, and `{@render
  children()}` in a `min-w-0 flex-1` content column.
- [x] 2.2 Add `web/src/routes/my/+page.ts` → `redirect(308, '/my/tracking')`.

## 3. Refactor pages to fit the shell

- [x] 3.1 `my/tracking/+layout.svelte`: drop the outer container, the
  `!isAuthenticated()` gate, and the `noindex` (now owned by `my/+layout`); keep
  the `<title>`, `<h1>Tracking`, the tab row, and `{@render children()}`.
- [x] 3.2 `my/profile/+page.svelte`: remove the outer `mx-auto max-w-6xl px-4
  py-6` wrapper, the `!isAuthenticated()` gate, and the per-page `noindex`; keep
  `<title>`, header, and the internal tabbed layout.
- [x] 3.3 `my/recommendations/+page.svelte`: remove outer wrapper + gate +
  noindex; body fills the content column.
- [x] 3.4 `my/searches/+page.svelte`: remove outer wrapper + gate + noindex;
  keep an inner `max-w-3xl` on the body for reading width.
- [x] 3.5 `my/api-keys/+page.svelte`: remove outer wrapper + gate + noindex;
  keep inner `max-w-3xl` on the body.
- [x] 3.6 `my/submissions/+page.svelte`: remove outer wrapper + gate + noindex;
  keep inner `max-w-3xl` on the body.

## 4. Verify

- [x] 4.1 `npm run check` (svelte-check), eslint, and vitest all green.
- [x] 4.2 Visual verify: signed-in shows the sidebar at `lg`+ and the strip below
  `lg` with the correct active item per section; signed-out shows the single
  gate; `/my` redirects to `/my/tracking`; Tracking sub-tabs still work.
