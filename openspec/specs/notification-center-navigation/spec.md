# notification-center-navigation Specification

## Purpose

The tabbed route structure for the `/my/notifications` section — one shared
tab strip over three real, navigable routes (History, Search alerts,
Settings) — and the redirect from the section's retired history URL.

## Requirements

### Requirement: Notification center tab structure

The `/my/notifications` section SHALL render a shared tab strip above its
content with three tabs — History, Search alerts, Settings — each backed by
its own real, navigable route: `/my/notifications` (History),
`/my/notifications/searches` (Search alerts), `/my/notifications/settings`
(Settings). Selecting a tab SHALL navigate to its route rather than only
swapping client-side view state. The tab whose route matches (or is the
longest-matching ancestor of) the current path SHALL be marked active.

#### Scenario: History is the section's landing page

- **WHEN** a signed-in user opens `/my/notifications`
- **THEN** the page shows the notification delivery history with the History
  tab marked active

#### Scenario: Selecting a tab navigates

- **WHEN** a signed-in user on `/my/notifications` selects the Settings tab
- **THEN** the app navigates to `/my/notifications/settings` and shows that
  tab as active

#### Scenario: Digest detail page keeps the History tab active

- **WHEN** a signed-in user opens a notification history card's matched-jobs
  page at `/my/notifications/:id/jobs`
- **THEN** the shared tab strip renders with the History tab marked active

### Requirement: Retired history URL redirects

The URL `/my/notifications/history` SHALL redirect (308) to
`/my/notifications`.

#### Scenario: Old history URL redirects

- **WHEN** any visitor requests `/my/notifications/history`
- **THEN** the app redirects (308) to `/my/notifications`
