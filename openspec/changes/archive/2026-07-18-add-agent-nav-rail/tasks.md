## 1. Shared account-nav icon map

- [x] 1.1 Create `web/src/lib/accountNavIcons.ts` exporting `accountNavIcons: Record<AccountNavItem['href'], LucideIcon>` (moved verbatim from `my/+layout.svelte`). The "icon for every href" invariant is enforced by the `Record<AccountNavItem['href'], …>` type (missing key = compile error, extra key = excess-property error) and verified by `svelte-check` — a vitest cannot cover it because this repo's vitest does not transform `.svelte` imports (the reason `accountNav.ts` is kept Svelte-free). Prove the guard by momentarily dropping a key and seeing `svelte-check` fail, then restore.
- [x] 1.2 Update `web/src/routes/my/+layout.svelte` to import `accountNavIcons` from the shared module and drop the inline map; confirm the sidebar/tab strip still renders unchanged.

## 2. AccountNavRail component

- [x] 2.1 Create `web/src/lib/components/AccountNavRail.svelte`: signed-in gate (renders nothing when signed out), items from `visibleAccountNav(role/beta)`, active via `isSectionActive(page.url.pathname, href)`, one icon-only `<a>` per item with `title` tooltip, `aria-current`, matching the sidebar's active/hover item classes.
- [x] 2.2 Style it as a fixed-width (`w-14`), `shrink-0`, full-height right-edge column with a left border, centring each icon.

## 3. Wire into the Agent page

- [x] 3.1 Render `<AccountNavRail>` as the last flex child in `web/src/routes/my/assistant/+page.svelte` so it pins to the right edge beside `<AssistantChat>`.

## 4. Verify

- [x] 4.1 `npm run check` (svelte-check) and the build pass in `web/`.
- [x] 4.2 Visual-verify `/my/assistant` (signed-in beta user) in headless Chrome: rail on the right edge, correct icons, Agent item active, tooltips present; and signed-out shows no rail.
