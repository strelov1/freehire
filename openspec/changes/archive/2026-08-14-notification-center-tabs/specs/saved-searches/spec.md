## MODIFIED Requirements

### Requirement: Saved searches section in the account area

The web app SHALL expose a dedicated account section at
`/my/notifications/searches`, reachable as a tab of the Notifications section
from the header account menu alongside the other `/my/*` sections, that lists
the signed-in user's saved searches and lets them manage each one: share it as
a public board (with an optional author label), unshare it, copy its public
`/b/:slug` link when shared, rename it, and delete it. Creating a new saved
search is out of scope for this section (it happens in the filters "My
filters" control where the current filters exist). An anonymous visitor SHALL
be prompted to sign in rather than shown a list. The retired
`/my/searches` URL SHALL redirect to `/my/notifications/searches`.

#### Scenario: List and manage from the account section

- **WHEN** a signed-in user opens `/my/notifications/searches`
- **THEN** the page lists their saved searches, each showing whether it is shared, with actions to share, unshare, rename, delete, and (when shared) copy its public link

#### Scenario: Share from the account section

- **WHEN** a signed-in user shares a saved set from `/my/notifications/searches` (optionally supplying an author label) and confirms
- **THEN** the set is marked shared and its copyable public `/b/:slug` link is surfaced

#### Scenario: Unshare from the account section

- **WHEN** a signed-in user unshares a shared set from `/my/notifications/searches`
- **THEN** the set is marked private again and its public link is no longer offered

#### Scenario: Anonymous access to the section

- **WHEN** an anonymous (signed-out) visitor opens `/my/notifications/searches`
- **THEN** the page prompts sign-in instead of listing saved searches

#### Scenario: Old URL redirects

- **WHEN** any visitor requests `/my/searches`
- **THEN** the app redirects (308) to `/my/notifications/searches`
