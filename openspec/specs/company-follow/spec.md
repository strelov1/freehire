# company-follow Specification

## Purpose
TBD - created by archiving change add-company-follow-button. Update Purpose after archive.
## Requirements
### Requirement: Company page exposes a follow-for-updates action

The company page SHALL present a "Subscribe to updates" control that lets a user
follow the company's new job postings, delivered through the existing
saved-search and Telegram filter-subscription capabilities. The control SHALL
reflect the current follow state and act as a toggle.

#### Scenario: Control is shown to a follow-capable user

- **WHEN** a signed-in user whose Telegram integration is enabled server-side
  opens a company page
- **THEN** a "Subscribe to updates" button is rendered in the company header

#### Scenario: Control reflects the not-following state

- **WHEN** the user is not following the company
- **THEN** the button reads "Subscribe to updates"

#### Scenario: Control reflects the following state

- **WHEN** the user already follows the company (a saved search with query
  `company_slug=<slug>` has an active Telegram subscription)
- **THEN** the button reads "Subscribed"

### Requirement: Following a company reuses saved-search and subscription primitives

Following a company SHALL be expressed as a saved search whose query is exactly
`company_slug=<slug>` plus a Telegram subscription on that saved search. The
system SHALL reuse an existing saved search that already carries that canonical
query rather than creating a duplicate; otherwise it SHALL create one named after
the company. No new backend capability is introduced.

#### Scenario: Follow creates the saved search and subscription

- **WHEN** a follow-capable user with a linked Telegram taps "Subscribe to
  updates" and no saved search for this company exists
- **THEN** a saved search with query `company_slug=<slug>` named after the company
  is created, and a Telegram subscription is created on it

#### Scenario: Follow reuses an existing matching saved search

- **WHEN** the user already has a saved search whose canonical query equals
  `company_slug=<slug>` and taps "Subscribe to updates"
- **THEN** the existing saved search is reused (no duplicate is created) and a
  Telegram subscription is created on it

### Requirement: Unfollowing is a clean toggle that preserves user-owned filters

Unfollowing SHALL delete the Telegram subscription. It SHALL also delete the
saved search only when that saved search is the one the follow action generated —
identified by its name matching the company name. A saved search the user created
and named themselves SHALL NOT be deleted by unfollowing.

#### Scenario: Unfollow removes the generated saved search

- **WHEN** the user unfollows a company whose saved search name equals the company
  name (the generated follow filter)
- **THEN** the Telegram subscription is deleted and the saved search is deleted

#### Scenario: Unfollow preserves a user-named filter

- **WHEN** the user unfollows a company whose subscription rides on a saved search
  the user named differently from the company name
- **THEN** the Telegram subscription is deleted but the saved search is kept

### Requirement: Follow action handles auth and Telegram-linking preconditions

The follow control SHALL guide the user through the required preconditions instead
of failing. A signed-out user SHALL be routed to sign in. A signed-in user without
a linked Telegram SHALL be walked through the same deep-link connect flow used by
the "My filters" panel before the subscription is created. When the Telegram bot
is not configured server-side, the control SHALL be hidden.

#### Scenario: Signed-out user is prompted to sign in

- **WHEN** a signed-out user activates the follow control
- **THEN** the authentication dialog is opened and no subscription is attempted

#### Scenario: Unlinked Telegram triggers the connect flow

- **WHEN** a signed-in user whose Telegram is not linked taps "Subscribe to
  updates"
- **THEN** the Telegram connect deep link is opened and a re-check affordance is
  shown, rather than creating a subscription immediately

#### Scenario: Control hidden when Telegram is disabled server-side

- **WHEN** a signed-in user opens a company page and the Telegram bot is not
  configured server-side
- **THEN** the follow control is not rendered

