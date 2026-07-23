## Context

The workspace lays out three columns, each with its own selector: the left
panel toggles `leftTab` (Editor/Chat); the centre is the always-on preview; the
right `ArtifactPanel` owns its own tab (Templates/Job/Verdict). On desktop all
three are visible at once, so two independent selectors + the always-on preview
coexist. On mobile only one view fits at a time, which needs a *single* choice
across all six — a different shape from the desktop's two-independent-selectors
model.

## Decision

Add one page-level state, `mobileView`
(`'chat' | 'editor' | 'preview' | 'templates' | 'jd' | 'verdict'`), as the sole
source of truth for per-region visibility **on mobile**. Desktop ignores it —
`lg:flex` re-shows every region regardless of the mobile `hidden`.

`pickMobile(v)` sets `mobileView` and syncs the matching column's existing
selector (`leftTab` for chat/editor, the lifted `artifactTab` for
templates/jd/verdict) so desktop and mobile never disagree. The centre preview
has no sub-selector, so `preview` only toggles visibility.

Region visibility uses the existing repo pattern already proven in this file:
a conditional `flex`/`hidden` plus a static `lg:flex`. At `lg` the `lg:flex`
utility wins (Tailwind emits responsive variants after base utilities, and the
`@media` rule wins at equal specificity as the later rule), so every column is
shown; below `lg` the conditional class decides.

`ArtifactPanel`'s tab is lifted to a `$bindable` prop so the page's flat bar can
drive it while its own desktop tab bar keeps setting it via the same binding.
Its fixed pixel width moves from an inline `width:` to a `--w` CSS variable
(`w-full lg:w-[var(--w)]`), mirroring how the left panel already handles its
width, so the panel fills the screen on mobile rather than staying a narrow
column.

## Alternatives considered

- **Two-level mobile tabs** (3 primary column tabs, nested sub-tabs inside
  Edit/Context): fewer top-level tabs but nested navigation on a small screen;
  rejected for the flat six-tab bar per the product call.
- **Unifying all selection into one enum** (dropping `leftTab`/`artifactTab`):
  breaks the desktop model where two selectors must be independently active at
  once. Rejected — `mobileView` layered on top keeps desktop untouched.

## Known limitation

The "Saving/Saved" indicator sits in the left panel's header, which is
desktop-only, so it does not render on mobile. Autosave itself is unaffected.
Restoring a mobile indicator (e.g. in the flat bar) is a deliberate follow-up,
left out to keep the change minimal.
