## 1. Restructure the menu panel

- [x] 1.1 Extract the theme toggle + auth action (Sign in / Log out) into a Svelte
  `{#snippet}` so they can be rendered in two places without duplicating logic
- [x] 1.2 Convert the panel container to two layouts via responsive classes: mobile
  `fixed inset-0 flex flex-col` full-screen drawer; desktop the existing anchored
  `w-64` dropdown (unchanged look)

## 2. Mobile drawer regions

- [x] 2.1 Add the mobile-only top bar (`sm:hidden`): brand wordmark + close control
  that sets `open = false`
- [x] 2.2 Make the link list the scrollable middle region (`max-sm:flex-1
  max-sm:overflow-y-auto`) with labelled sections (Navigate / Account) and ≥44px
  (`min-h-11`, `text-base`) tap targets; keep item gating (account, moderator) intact
- [x] 2.3 Add the mobile-only pinned bottom bar (`sm:hidden`) rendering the theme/auth
  snippet, separated by a top border; render the same snippet inline for desktop
  (`max-sm:hidden`)

## 3. Behavior + verify

- [x] 3.1 Preserve scroll-lock and close-on-select/`Escape`; adjust the mobile backdrop
  handling for the full-screen sheet (opaque sheet; close via top-bar control / Escape /
  outside-click on desktop)
- [x] 3.2 `svelte-check` and eslint pass for `HeaderMenu.svelte`
- [x] 3.3 Visual check: mobile drawer (signed-in and signed-out) and desktop dropdown
  render correctly in light and dark themes; tap targets look ≥44px; no overflow
