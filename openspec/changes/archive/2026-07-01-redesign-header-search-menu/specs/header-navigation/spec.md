## ADDED Requirements

### Requirement: Unified header layout

The site-wide header SHALL present three slots in a single row — the logo (left,
linking to `/`), a large search field (center, growing to fill available width),
and a single menu trigger button (right) — using the same structure on every
viewport. The header SHALL NOT render inline nav links or a separate avatar
dropdown outside the menu.

#### Scenario: Header renders three slots on desktop

- **WHEN** a user loads any page on a wide viewport
- **THEN** the header shows the logo, a centered search field, and one menu
  trigger button, with no inline nav links or standalone avatar control

#### Scenario: Header renders three slots on mobile

- **WHEN** a user loads any page on a narrow viewport
- **THEN** the header shows the logo, the search field, and one menu trigger
  button in the same arrangement as desktop

### Requirement: Instant search results

The header search SHALL query as the user types (debounced) and render a
dropdown with up to two sections — **JOBS** (each item showing title, company,
and location) and **COMPANIES** (each item showing name and job count) — sourced
from the existing jobs-search and companies-list APIs. An in-flight query
SHALL be superseded by a newer one so stale results never overwrite fresh ones.
When the query is empty the dropdown SHALL be closed; when a non-empty query
matches nothing the dropdown SHALL show an empty state.

#### Scenario: Typing shows job and company matches

- **WHEN** a user types a query that matches jobs and companies
- **THEN** the dropdown opens with a JOBS section listing matching vacancies
  (title, company, location) and a COMPANIES section listing matching companies
  (name, job count)

#### Scenario: Query with no matches

- **WHEN** a user types a query that matches neither jobs nor companies
- **THEN** the dropdown shows an empty state instead of sections

#### Scenario: Clearing the query closes the dropdown

- **WHEN** a user clears the search field
- **THEN** the dropdown closes and no results are shown

### Requirement: Search keyboard control and hotkeys

The header search SHALL be focusable from anywhere via the `Cmd/Ctrl+K` and `/`
hotkeys (the `/` hotkey ignored while another text input is focused). Within the
open dropdown, `ArrowUp`/`ArrowDown` SHALL move the active result, `Enter` SHALL
open the active result, and `Escape` SHALL close the dropdown. With no active
result, `Enter` on a non-empty query SHALL navigate to `/jobs?q=<query>`.

#### Scenario: Hotkey focuses the search

- **WHEN** a user presses `Cmd/Ctrl+K` (or `/` while not typing in a field)
- **THEN** the header search field receives focus

#### Scenario: Arrow keys and Enter open a result

- **WHEN** the dropdown is open and the user presses `ArrowDown` then `Enter`
- **THEN** the highlighted result is opened via navigation

#### Scenario: Enter with no active result runs a full search

- **WHEN** a user has typed a non-empty query with no highlighted result and
  presses `Enter`
- **THEN** the app navigates to `/jobs?q=<query>`

#### Scenario: Escape closes the dropdown

- **WHEN** the dropdown is open and the user presses `Escape`
- **THEN** the dropdown closes

### Requirement: Search result targets

Selecting a JOBS result SHALL navigate to that job's detail page
(`/jobs/:slug`); selecting a COMPANIES result SHALL navigate to that company's
page (`/companies/:slug`). A click outside the search SHALL close the dropdown.

#### Scenario: Selecting a job opens its detail page

- **WHEN** a user selects a job from the dropdown
- **THEN** the app navigates to `/jobs/<public_slug>` for that job

#### Scenario: Selecting a company opens its page

- **WHEN** a user selects a company from the dropdown
- **THEN** the app navigates to `/companies/<slug>` for that company

#### Scenario: Outside click closes the dropdown

- **WHEN** the dropdown is open and the user clicks outside the search
- **THEN** the dropdown closes

### Requirement: Consolidated menu

The single menu SHALL contain the site nav links (Jobs, Companies, Collections,
Analytics, CLI, For recruiters, For companies), the theme toggle, and the auth
action. For a signed-in user the menu SHALL additionally contain the account
items (My jobs, Search profiles, Notifications, API keys, Submit a job, My
submissions) with a Log out action; the Moderation item SHALL appear only for a
moderator. For a signed-out user the menu SHALL show a Sign in action instead of
the account items. The menu SHALL close after an item is selected, on `Escape`,
and on outside click.

#### Scenario: Signed-out menu

- **WHEN** a signed-out user opens the menu
- **THEN** it shows the nav links, the theme toggle, and a Sign in action, and
  no account items

#### Scenario: Signed-in menu

- **WHEN** a signed-in non-moderator user opens the menu
- **THEN** it shows the nav links, the account items (My jobs, Search profiles,
  Notifications, API keys, Submit a job, My submissions), the theme toggle, and
  Log out, but not Moderation

#### Scenario: Moderator sees moderation

- **WHEN** a signed-in moderator opens the menu
- **THEN** the menu additionally shows the Moderation item

#### Scenario: Selecting an item closes the menu

- **WHEN** the menu is open and the user selects any link or action
- **THEN** the menu closes

### Requirement: Active nav indication

A nav link in the menu SHALL be marked active when the current path equals its
target or is a sub-route of it, so the menu reflects where the user is.

#### Scenario: Current section is highlighted

- **WHEN** the user is on `/jobs` or `/jobs/:slug` and opens the menu
- **THEN** the Jobs link is shown as active

### Requirement: Mobile overlay behavior

On a narrow viewport the open search dropdown and the open menu SHALL render as
full-width overlays with a backdrop, and body scroll SHALL be locked while
either overlay is open so background content does not scroll behind it.

#### Scenario: Menu locks background scroll on mobile

- **WHEN** a user opens the menu on a narrow viewport
- **THEN** a backdrop appears and the page body behind it does not scroll

#### Scenario: Search dropdown locks background scroll on mobile

- **WHEN** the search dropdown is open on a narrow viewport
- **THEN** a backdrop appears and the page body behind it does not scroll
