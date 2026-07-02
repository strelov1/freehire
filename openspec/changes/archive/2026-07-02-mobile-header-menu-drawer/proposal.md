## Why

On mobile the header menu (the hamburger panel) is a desktop dropdown stretched to
full width: cramped `py-2` rows (tap targets ~32–36px, below the 44px guideline), a
flat undifferentiated list of nav + account + theme + auth items separated only by
hair-lines, and a `max-h-[80vh]` overlay that leaves a strip of backdrop at the
bottom. It reads as unpolished. The menu's *contents* are fine; its mobile
*presentation* is the problem.

## What Changes

- Redesign the menu's **mobile** presentation into a full-screen **drawer**:
  - Its own top bar (brand wordmark + a close button) so it reads as a screen, not a
    dropdown.
  - A scrollable middle region with the links grouped into labelled sections
    (Navigate / Account) and **larger tap targets** (≥48px rows, `text-base`).
  - The theme toggle and the auth action **pinned to a bottom bar** (thumb-reachable),
    separated by a top border.
- Keep the existing **desktop** anchored dropdown unchanged.
- Preserve all current behavior: same menu items and gating (nav, account, moderator,
  sign in/out), close on item-select / `Escape` / close-control, and body-scroll lock
  while open on mobile.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `header-navigation`: the menu's mobile overlay becomes a full-screen drawer with a
  top bar, sectioned larger-tap-target links, and a pinned bottom action bar (the menu
  *contents* and desktop dropdown are unchanged).

## Impact

- `web/src/lib/components/HeaderMenu.svelte` — the only file changed (markup + Tailwind
  classes; a Svelte snippet to avoid duplicating the theme/auth actions across the
  mobile bottom bar and desktop inline list).
- No backend, API, or DB impact. No new dependencies. Verified via `svelte-check` +
  eslint + visual check (no web test runner).
