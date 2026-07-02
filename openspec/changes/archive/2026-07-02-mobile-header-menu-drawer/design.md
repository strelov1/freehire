## Context

`HeaderMenu.svelte` renders one panel that adapts by breakpoint: `max-sm` → a
full-width, `max-h-[80vh]` overlay dropping from `top-14` with a backdrop; `sm` → an
anchored `w-64` dropdown. The item list (email, nav links, account links, moderation,
theme toggle, auth) is a flat sequence of `px-3 py-2 text-sm` rows split by `h-px`
dividers. Body scroll is locked on mobile via `lockScroll`/`unlockScroll`; the menu
closes on `afterNavigate`, `Escape`, outside click, and item select. The design system
is flat-neutral (semantic tokens, system fonts); no web test runner.

## Goals / Non-Goals

**Goals:**
- Mobile menu reads as a proper full-screen drawer: brand/close top bar, sectioned
  scrollable links with ≥44px tap targets, theme+auth pinned to a bottom bar.
- Desktop dropdown unchanged in look and behavior.
- No change to menu items, gating, or close/scroll-lock behavior.

**Non-Goals:**
- No new deps, animation library, colors, or fonts.
- No change to the search field/dropdown, the header layout, or account routes.

## Decisions

- **Single component, two layouts via responsive classes** (not a second component).
  The container is `max-sm:fixed max-sm:inset-0 max-sm:flex max-sm:flex-col` (full-screen
  column) and `sm:absolute sm:right-0 sm:top-full sm:w-64 sm:rounded-md` (dropdown).
  Alternative — split mobile/desktop components — rejected: it would duplicate the item
  list and its gating logic.
- **Three flex regions on mobide, collapsing to a plain list on desktop.** Mobile top
  bar and pinned bottom bar are `sm:hidden`; the middle link region is `max-sm:flex-1
  max-sm:overflow-y-auto`. On desktop the middle region is the whole dropdown
  (`sm:max-h-[80vh] sm:overflow-y-auto`) and theme/auth render inline at its end.
- **Theme + auth defined once via a Svelte `{#snippet}`**, rendered in the mobile
  bottom bar (`sm:hidden`) and again inline for desktop (`max-sm:hidden`). This satisfies
  the spec's "defined once, no duplicated logic" without forcing one visual placement on
  both breakpoints. Chosen over duplicating the two buttons.
- **Tap targets** use `min-h-11` (44px) + `text-base` on drawer rows; section labels are
  `text-xs uppercase tracking-wider text-muted-foreground`. Chosen to meet the 44px
  guideline while staying within existing tokens.
- **Full-screen `inset-0` (covers the header) with its own top bar** rather than
  `top-14`. Removes the leftover-backdrop strip and the double-header look; the drawer's
  own brand+close replaces the covered header controls. No separate backdrop needed on
  mobile since the sheet is opaque and full-screen (desktop keeps outside-click close).

## Risks / Trade-offs

- [No web test runner → scenarios can't be unit-tested] → verify with `svelte-check`,
  eslint, and visual checks (mobile open drawer signed-in/out, desktop dropdown), both
  themes.
- [Responsive-class divergence is easy to get subtly wrong] → verify BOTH breakpoints
  visually, not just mobile.
- [Full-screen inset-0 removes the mobile backdrop] → acceptable: the opaque sheet is the
  backdrop; scroll-lock still applies; close via the top-bar control / `Escape` /
  item-select.

## Migration Plan

Pure frontend edit to one component. No data/flags. Rollback = revert the commit.
