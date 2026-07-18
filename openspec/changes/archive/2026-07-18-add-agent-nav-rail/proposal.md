## Why

The Agent chat page (`/my/assistant`) is a full-width, app-like surface that
deliberately resets past the `my/*` account shell (`+layout@.svelte`) to reclaim
horizontal space. The side effect is that it loses the account-section
navigation entirely — from the Agent page there is no way to jump to Profile,
Tracking, Inbox, CV builder, notifications, keys, etc. without going back through
the header menu. We want that navigation back on this surface, but in a compact
form that does not steal width from the chat.

## What Changes

- Add a narrow, icon-only account-navigation rail pinned to the far-right edge of
  the Agent page (`/my/assistant`).
- The rail lists the same sections as the account sidebar (`visibleAccountNav`),
  honouring the same beta/moderator gating, in the same order, with the same
  Lucide icons.
- Each item is icon-only (no text label), carries a `title` tooltip with its
  label, links to its `my/*` route, and is marked active by the existing
  `isSectionActive` rule.
- The rail is always narrow (~`w-14`); no expand/collapse and no persistence.
- Extract the current inline href→icon map from `my/+layout.svelte` into a shared
  module so the left sidebar and the new rail share one source of truth (a new
  section without an icon stays a compile error in both places).
- Scope is `/my/assistant` only; the `/tailor` surface is unchanged.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `account-navigation`: add a requirement that the full-width Agent surface,
  which opts out of the account shell, still presents the account section
  navigation as a compact icon-only rail pinned to the right edge.

## Impact

- `web/src/routes/my/assistant/+page.svelte` — renders the new rail beside the
  chat.
- New `web/src/lib/components/AccountNavRail.svelte` — the icon-only rail.
- New shared module for the href→icon map (e.g. `web/src/lib/accountNavIcons.ts`),
  consumed by both `my/+layout.svelte` and the rail.
- `web/src/routes/my/+layout.svelte` — imports the icon map from the shared
  module instead of defining it inline.
- No backend, API, or data changes; reuses `$lib/accountNav`
  (`visibleAccountNav`, `isSectionActive`) and existing auth state.
