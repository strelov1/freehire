## ADDED Requirements

### Requirement: Full-width surface navigation rail

The Agent surface SHALL present the account section navigation as a compact,
icon-only rail pinned to the far-right edge. The Agent page (`/my/assistant`)
opts out of the account shell and would otherwise have no section navigation. The
rail SHALL list exactly the sections returned by the shared visible
navigation model — the same items, order, and beta/moderator gating as the
account sidebar — and SHALL NOT include create actions or non-account links. Each
rail item SHALL render its section's icon with no text label, expose its section
label as a hover tooltip, link to its `my/*` route, and be marked active by the
same rule as the account sidebar (active when the current path equals the
section's route or is a descendant of it). The rail SHALL be shown only to a
signed-in user.

#### Scenario: Agent page shows the icon-only rail

- **WHEN** a signed-in user opens `/my/assistant`
- **THEN** a compact icon-only navigation rail is pinned to the right edge,
  listing the same account sections (with the same gating) as the account sidebar

#### Scenario: Rail item reflects the active section

- **WHEN** a signed-in user is on `/my/assistant`
- **THEN** the Agent item in the rail is marked active and the other items are not

#### Scenario: Rail item exposes its label on hover

- **WHEN** a user hovers a rail item that shows only an icon
- **THEN** the item's section label is available as a tooltip

#### Scenario: Signed-out visitor sees no rail

- **WHEN** a signed-out visitor opens `/my/assistant`
- **THEN** the navigation rail is not rendered
