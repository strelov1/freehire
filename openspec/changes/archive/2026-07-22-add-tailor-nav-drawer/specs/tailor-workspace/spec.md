## ADDED Requirements

### Requirement: The account nav collapses to a drawer on mobile

The tailoring workspace SHALL, below the `lg` breakpoint, hide the fixed account
icon rail and instead expose the account sections through a burger button in the
mobile tab bar that opens a labelled slide-in drawer over a dimmed backdrop. The
drawer SHALL close on backdrop click, on `Escape`, on its close button, and after
a nav link is followed. At `lg` and up the account icon rail SHALL render as
before and no burger SHALL be shown.

#### Scenario: The burger opens the account drawer on mobile

- **WHEN** the workspace renders below `lg` and the user taps the burger in the mobile tab bar
- **THEN** a labelled drawer of account sections slides in over a dimmed backdrop, and the fixed icon rail is not shown

#### Scenario: The drawer dismisses

- **WHEN** the drawer is open and the user taps the backdrop, presses `Escape`, taps the close button, or follows a link
- **THEN** the drawer closes

#### Scenario: The rail is unchanged at lg

- **WHEN** the workspace renders at `lg` or wider
- **THEN** the account icon rail shows on the left edge as before and no burger button is present
