## 1. AccountNavRail — opt-in collapsible drawer

- [x] 1.1 Add `open = $bindable(false)` and `collapsible = false` props; extract the rail link into a shared snippet (icon-only vs icon+label).
- [x] 1.2 With `collapsible`: render the desktop icon rail `hidden … lg:flex`, and below `lg` a backdrop + labelled slide-in drawer gated on `open`; close on backdrop/Escape/close-button/link.
- [x] 1.3 Without `collapsible`: render the rail exactly as before (assistant unaffected).

## 2. Tailor page — burger trigger

- [x] 2.1 Add `navOpen` state; render a burger button at the start of the mobile tab bar toggling it; pass `collapsible bind:open={navOpen}` to `AccountNavRail`.

## 3. Verify

- [x] 3.1 `npm run check` — 0 errors; `eslint` — clean on both files.
- [ ] 3.2 Visual-verify on mobile (throwaway route + headless Chrome): burger opens the drawer, backdrop/Escape/link close it, desktop rail unchanged at `lg`, `/my/assistant` rail unchanged.
