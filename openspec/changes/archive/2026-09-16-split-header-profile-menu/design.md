## Context

`HeaderMenu.svelte` currently renders one `open` state and one dropdown panel
whose markup is shared, via responsive Tailwind classes, between a mobile
full-screen drawer and a desktop anchored dropdown. Mutual exclusivity between
header overlays (this menu, the notification bell, search suggestions) is
already handled by `$lib/headerOverlay.ts` (`openedOverlay`/`closedOverlay`),
which is written to support any number of overlays and needs no change. See
proposal.md - Why/What Changes for the motivation and desired end state.

## Goals / Non-Goals

**Goals:**

- Desktop signed-in users get a second, independent dropdown off the profile
  icon, scoped to their own account, with `Log out` visible without scrolling.
- Mobile behavior (single combined drawer) is pixel-for-pixel unchanged.
- Reuse the existing overlay-coordination and close-on-navigation/outside
  click/Escape patterns rather than inventing new ones.

**Non-Goals:**

- No change to `TopBar.svelte`'s three-slot composition (logo | search |
  menu) — the new trigger is composed inside the menu slot's existing
  component, not a fourth slot.
- No redesign of the mobile drawer's structure, sections, or item set.
- No change to which items exist in `accountLinks` — only which dropdown
  renders them on which viewport. (`navLinks`/`NAV` does gain one entry, Open
  — see Decisions.)

## Decisions

**New component `HeaderProfileMenu.svelte`, not a second state machine inside
`HeaderMenu.svelte`.** The profile dropdown is desktop-only and has its own
trigger element (the existing profile icon), its own open state, and its own
outside-click/Escape handling — the same shape `HeaderMenu.svelte` already
has for the consolidated menu. Duplicating that shape in a sibling component
is simpler than threading a second `open`/`root` pair and a second panel
through the existing component's already-branchy mobile/desktop markup.
Alternative considered: add a second `open2` state directly in
`HeaderMenu.svelte`. Rejected — the file is already 400+ lines mixing two
viewport layouts in one template; a second independent overlay's markup and
handlers are easier to reason about (and to unit-test in isolation) as their
own file.

**`HeaderProfileMenu.svelte` is rendered from `HeaderMenu.svelte`, replacing
the current inline profile-icon/sign-in block**, not added as a new sibling
in `TopBar.svelte`. `TopBar.svelte`'s own comment states the menu slot is
"one cluster, not two" (bell lives inside `HeaderMenu`); keeping the new
trigger inside `HeaderMenu.svelte` preserves that composition boundary from
`TopBar`'s point of view — `TopBar` still renders a single `<HeaderMenu />`.

**The account-items block's markup stays inline in `HeaderMenu.svelte`,
hidden on desktop via `sm:hidden`; only its data (`accountLinks`) is
extracted.** The mobile drawer's rendering of Profile/`accountLinks`/Submit a
job/Moderation is correct as-is — wrapping that existing block in `sm:hidden`
removes it from the desktop rendering without touching its markup. But the
new `HeaderProfileMenu.svelte` needs the same item list for its own,
differently-styled panel, and `accountLinks` was a `const` local to
`HeaderMenu.svelte` — implementing 1.1/1.2 surfaced that duplicating the
seven-item array in both files would recreate exactly the drift `siteNav.ts`'s
own comment warns about ("two arrays... went on pointing at the old one").
So `accountLinks` (and its type) moves to a new `$lib/headerAccountLinks.ts` —
a plain data module, Svelte-free like `siteNav.ts` and `accountNavIcons.ts`,
kept deliberately separate from `accountNavIcons.ts`'s full account-rail map
since this is a smaller, differently-curated subset for the header only (see
`HeaderMenu.svelte`'s existing comment on `accountLinks`) — imported by both
components. Each component keeps its own markup/styling for the shared data;
only the item list itself is de-duplicated.

**Desktop `☰` menu drops the auth snippet (`authButton`) entirely**, since
`Log out` moves to `HeaderProfileMenu` and signed-out `Sign in` already has a
direct icon action in the controls cluster today. The theme toggle
(`themeButton`) stays in the desktop `☰` menu — it is not profile-scoped.

**Overlay coordination**: `HeaderProfileMenu` calls `openedOverlay`/
`closedOverlay` exactly as `HeaderMenu` does today, so opening either menu (or
the bell) closes whichever of the other two was open, with no new
coordination logic needed.

**`Open` is added to `NAV` in `siteNav.ts`, not just spelled inline in
`HeaderMenu.svelte`.** `siteNav.ts`'s own comment explains why the destinations
live in one shared table: `HeaderMenu` and `TopBar`'s homepage row
(`HEADER_LINKS`) both draw from it, and a destination added only in one place
is how they drifted before. `HEADER_LINKS` stays the five entries it lists
today — it is a deliberate five-item subset ("no more"), not "everything in
NAV", so adding `Open` to `NAV` does not add it there. In `HeaderMenu.svelte`,
`Open` renders right after `About` (both read from `NAV`, `About` already
rendered on its own at the foot of the nav-links list), for both viewports —
it is ordinary site nav, not part of the profile/consolidated split this
change is otherwise about.

## Risks / Trade-offs

- [Two components now read overlapping pieces of `$lib/auth.svelte` and
  `$lib/siteNav`] → both already export what's needed (`isAuthenticated`,
  `currentUser`, `NAV`); no new exports required, and each component only
  imports what it renders.
- [A visitor who resizes across the `sm` breakpoint while a menu is open]
  → out of scope: the existing single-menu implementation has the same
  gap today (no resize listener), so this is pre-existing behavior, not a
  regression.
