## MODIFIED Requirements

### Requirement: Mobile overlay behavior

On a narrow viewport the open search dropdown SHALL render as a full-width overlay
with a backdrop, and the open menu SHALL render as a full-screen **drawer** covering
the viewport. While either overlay is open, body scroll SHALL be locked so background
content does not scroll behind it. The desktop (wide-viewport) menu SHALL remain an
anchored dropdown.

#### Scenario: Menu opens as a full-screen drawer on mobile

- **WHEN** a user opens the menu on a narrow viewport
- **THEN** the menu covers the viewport as a full-screen drawer (not a partial-height
  dropdown) and the page body behind it does not scroll

#### Scenario: Menu stays a dropdown on desktop

- **WHEN** a user opens the menu on a wide viewport
- **THEN** the menu renders as an anchored dropdown near the trigger, not a full-screen
  drawer

#### Scenario: Search dropdown locks background scroll on mobile

- **WHEN** the search dropdown is open on a narrow viewport
- **THEN** a backdrop appears and the page body behind it does not scroll

## ADDED Requirements

### Requirement: Mobile menu drawer structure

The mobile menu drawer SHALL present three regions: a top bar with the brand wordmark
and an explicit close control; a scrollable middle region listing the menu items
grouped into labelled sections (a navigation section and, for a signed-in user, an
account section); and a bottom bar, pinned to the bottom of the drawer, holding the
theme toggle and the auth action (Sign in, or Log out for a signed-in user). Menu item
rows in the drawer SHALL have a touch target of at least 44px in height. The theme
toggle and auth action SHALL be defined once and reused across the mobile bottom bar
and the desktop dropdown (no duplicated logic).

#### Scenario: Drawer regions and sections

- **WHEN** a signed-in user opens the menu on a narrow viewport
- **THEN** the drawer shows a top bar with the brand and a close control, a scrollable
  list with a Navigate section and an Account section, and a pinned bottom bar with the
  theme toggle and Log out

#### Scenario: Larger tap targets

- **WHEN** the mobile drawer is open
- **THEN** each menu item row is at least 44px tall

#### Scenario: Close control dismisses the drawer

- **WHEN** the mobile drawer is open and the user taps its close control
- **THEN** the drawer closes

#### Scenario: Signed-out bottom bar

- **WHEN** a signed-out user opens the menu on a narrow viewport
- **THEN** the pinned bottom bar shows the theme toggle and a Sign in action, and the
  drawer shows no account section
