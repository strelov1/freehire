## Context

The account area (`my/*`) renders every route inside a shared shell
(`web/src/routes/my/+layout.svelte`) that owns the section navigation. The Agent
page opts out of that shell via `web/src/routes/my/assistant/+layout@.svelte`
(the `@` reset) so the chat runs full-width directly under the root layout — and
therefore has no section navigation at all.

The section navigation is driven by a pure model, `web/src/lib/accountNav.ts`
(`visibleAccountNav(isModerator, isBetaTester)` and `isSectionActive(path,
href)`), kept free of Svelte/icon imports. The href→Lucide-icon map currently
lives inline in `my/+layout.svelte`, typed as
`Record<AccountNavItem['href'], LucideIcon>` so a section without an icon is a
compile error.

## Goals / Non-Goals

**Goals:**
- Restore account section navigation on the Agent page as a compact, icon-only
  rail on the right edge.
- Reuse the existing navigation model and icon set verbatim — no second source of
  truth for the item list or the icons.

**Non-Goals:**
- No expand/collapse, no label text, no persisted state.
- No change to `/tailor` or any other `my/*` page.
- No change to the account model's ordering, gating, or active rule.

## Decisions

### Extract the href→icon map into a shared module

The rail needs the same href→icon map the sidebar uses. Duplicating the 10-entry
map in two components is a maintenance hazard: the compile-time "every section
has an icon" guarantee would only hold in one place. So move the map into a new
`web/src/lib/accountNavIcons.ts` exporting
`accountNavIcons: Record<AccountNavItem['href'], LucideIcon>`, and import it in
both `my/+layout.svelte` and the new rail. The map imports from `@lucide/svelte`,
so it lives in its own `.ts` module rather than in `accountNav.ts` — that keeps
`accountNav.ts` Svelte-free and unit-testable, as its comment requires.

_Alternative considered:_ pass the icon map into the rail as a prop from the page.
Rejected — it just relocates the duplication to the page and loses the shared
compile-time guarantee.

### A dedicated `AccountNavRail.svelte` component

Add `web/src/lib/components/AccountNavRail.svelte` that renders the icon-only
rail: it reads `currentUser()` for gating, computes items via `visibleAccountNav`,
resolves the active item with `isSectionActive` against `page.url.pathname`, and
renders one `<a>` per item (icon + `title` tooltip + `aria-current`). It renders
nothing when signed out. This mirrors the sidebar's item treatment (same
active/hover classes) without dragging in the sidebar's collapse logic, tab
strip, or width container.

_Alternative considered:_ inline the rail markup directly in the Agent page.
Rejected — a named component keeps the page a thin host and leaves a clean seam
if `/tailor` later wants the same rail.

### Pin the rail to the right edge of the Agent surface

The Agent page body is `<div class="flex h-[calc(100svh-3.5rem)]">` wrapping
`<AssistantChat>`. Render `<AccountNavRail>` as the last flex child so it sits to
the right of the chat column; the chat keeps `flex-1` / `min-w-0` and the rail is
a fixed-width (`w-14`), `shrink-0`, full-height column with a left border —
matching the visual language of the existing left session rail inside
`AssistantChat`. The rail scrolls independently if the section list ever exceeds
the viewport height.

## Risks / Trade-offs

- **Redundant with the header menu** (the hamburger already lists these) → the
  rail is a quick-access affordance specific to the full-width surface; this is an
  intentional, low-cost duplication of navigation, not of logic.
- **Narrow icon-only rail is less discoverable than labelled nav** → mitigated by
  `title` tooltips and by reusing the exact icons users already learned in the
  account sidebar.
- **Mobile width** → the Agent chat is already an app-like desktop-first surface;
  the rail is a thin `w-14` column that coexists with the chat. If it proves
  cramped on very small viewports we can hide it below a breakpoint, but that is
  out of scope for this change.
