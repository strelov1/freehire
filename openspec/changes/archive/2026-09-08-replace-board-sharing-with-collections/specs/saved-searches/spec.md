## REMOVED Requirements

### Requirement: Share a saved search as a public board
**Reason**: Public sharing of a saved search (a live query replay) is retired in favor of job lists, which share a fixed set of specific jobs instead of a query that drifts as the catalogue changes.
**Migration**: There is no automated migration — a query is not a set of jobs. A user who wants to share what a board showed should create a job list and add the specific jobs to it. Existing public board links stop resolving.

### Requirement: Unshare a public board
**Reason**: Retired along with sharing; a saved search's public/private state no longer exists once the feature is removed.
**Migration**: None needed — with sharing removed there is nothing left to unshare.

### Requirement: Public read of a shared board by slug
**Reason**: The `GET /api/v1/boards/:slug` endpoint is removed along with board sharing.
**Migration**: None — see job lists' public read endpoint for the replacement concept (specific jobs, not a query).

### Requirement: Public board page in the web app
**Reason**: The `/b/:slug` web route is removed along with board sharing.
**Migration**: See the job lists' `/l/:slug` public page for the replacement concept.

### Requirement: Saved searches section in the account area
**Reason**: The share/unshare/copy-link actions this requirement described no longer apply now that board sharing is removed; the account section's remaining behavior (list, rename, delete) is restated fresh, under a new requirement name, below — a `MODIFIED` block must carry every scenario the current spec has, and two of them (share/unshare) are being cut outright rather than adjusted, so this requirement is retired and replaced rather than edited in place.
**Migration**: None for the surviving actions (list/rename/delete are unchanged, see "Manage saved searches in the account area"). The share/unshare/copy-link affordances are simply removed from the page.

## ADDED Requirements

### Requirement: Manage saved searches in the account area

The web app SHALL expose a dedicated account section at
`/my/notifications/searches`, reachable as a tab of the Notifications section
from the header account menu alongside the other `/my/*` sections, that lists
the signed-in user's saved searches and lets them manage each one: rename it
and delete it. Creating a new saved search is out of scope for this section
(it happens in the filters "My filters" control where the current filters
exist). An anonymous visitor SHALL be prompted to sign in rather than shown a
list. The retired `/my/searches` URL SHALL redirect to
`/my/notifications/searches`.

#### Scenario: List and manage from the account section
- **WHEN** a signed-in user opens `/my/notifications/searches`
- **THEN** the page lists their saved searches, each with actions to rename and delete

#### Scenario: Anonymous access to the section
- **WHEN** an anonymous (signed-out) visitor opens `/my/notifications/searches`
- **THEN** the page prompts sign-in instead of listing saved searches

#### Scenario: Old URL redirects
- **WHEN** any visitor requests `/my/searches`
- **THEN** the app redirects (308) to `/my/notifications/searches`

## MODIFIED Requirements

### Requirement: List saved searches
A signed-in user SHALL be able to list their own saved searches, most recently updated first.

#### Scenario: List own sets
- **WHEN** an authenticated user sends `GET /api/v1/me/searches`
- **THEN** the system responds `200` with `{"data": [...]}` containing only that user's saved searches ordered by `updated_at` descending
