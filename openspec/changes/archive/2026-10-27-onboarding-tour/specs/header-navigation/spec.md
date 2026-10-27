## MODIFIED Requirements

### Requirement: Unified header layout

The site-wide header SHALL present three slots in a single row — the logo
(left, linking to `/`), a large search field (center, growing to fill
available width), and a controls cluster (right) — using the same structure
on every viewport. The header SHALL NOT render inline nav links outside the
controls cluster. On a wide viewport, the controls cluster additionally hosts
a profile menu trigger for a signed-in user (see Requirement: Desktop profile
menu); on a narrow viewport it does not, and the consolidated menu trigger is
the cluster's only menu control. For a signed-out visitor the controls cluster
SHALL show a labelled "Sign in" button on every viewport.

#### Scenario: Header renders three slots on desktop

- **WHEN** a user loads any page on a wide viewport
- **THEN** the header shows the logo, a centered search field, and the
  controls cluster (the consolidated menu trigger, plus a profile menu
  trigger when signed in), with no inline nav links outside that cluster

#### Scenario: Header renders three slots on mobile

- **WHEN** a user loads any page on a narrow viewport
- **THEN** the header shows the logo, the search field, and the controls
  cluster with a single menu trigger button, in the same arrangement as
  desktop

#### Scenario: Signed-out visitor sees Sign in

- **WHEN** a signed-out visitor loads any page on any viewport
- **THEN** the header's controls cluster shows a button labelled "Sign in"
