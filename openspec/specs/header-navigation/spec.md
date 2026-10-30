# header-navigation Specification

## Purpose

The site-wide header: its unified three-slot layout (logo, instant search,
consolidated menu), the search field's behavior (dropdown sections, keyboard
navigation, hotkeys, result targets), and the single menu's contents (nav links,
signed-in account items, theme toggle, auth actions) including mobile overlay
behavior.
## Requirements
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

### Requirement: Profile identity affordance

For a signed-in user the menu SHALL show an identity row consisting of an avatar
placeholder and the user's email. The avatar SHALL be a circle showing the first
character of the email, with a background colour deterministically derived from
the email (stable per user), and SHALL NOT require an uploaded image. The
identity row (avatar + email together) SHALL be a link to `/my/profile` and
selecting it SHALL close the menu and navigate there. For a signed-out user no
identity row SHALL be shown.

#### Scenario: Signed-in identity row links to the profile
- **WHEN** a signed-in user opens the menu
- **THEN** it shows an avatar (initial-in-a-circle) next to the user's email as a single clickable row, and selecting it closes the menu and navigates to `/my/profile`

#### Scenario: Avatar placeholder without an image
- **WHEN** a signed-in user with no uploaded picture opens the menu
- **THEN** the avatar shows the first character of their email on a colour derived from the email, and no broken/empty image is shown

#### Scenario: No identity row when signed out
- **WHEN** a signed-out user opens the menu
- **THEN** no avatar or email identity row is shown

### Requirement: Consolidated menu

The menu SHALL contain the site nav links (Jobs, Companies, Collections, the
site's feature pages, About, and Open) and the theme toggle. On a narrow
viewport the menu SHALL additionally contain, for a signed-in user, the
workspace switcher (Job search, then Mentor for a beta tester, then Referrer, then
Employer), a Settings
item, Submit a job, and a Log out action; the Moderation item SHALL appear only for a
moderator. For a signed-out user on a narrow viewport the menu SHALL show a Sign in action
instead of the account items. On a wide viewport the menu SHALL NOT contain the workspace
switcher, Settings, Submit a job, Moderation, or a Sign in/Log out action — those are
reached through the profile menu trigger (signed in) or the existing direct sign-in icon
(signed out); see Requirement: Desktop profile menu. The menu SHALL close after an item is
selected, on `Escape`, and on outside click.

#### Scenario: Signed-out menu on mobile

- **WHEN** a signed-out user opens the menu on a narrow viewport
- **THEN** it shows the nav links, the theme toggle, and a Sign in action, and
  no account items

#### Scenario: Signed-in menu on mobile

- **WHEN** a signed-in non-moderator user opens the menu on a narrow viewport
- **THEN** it shows the nav links, the workspace switcher, Settings, Submit a job, the
  theme toggle, and Log out, but not Moderation

#### Scenario: Profile item targets the single profile on mobile

- **WHEN** a signed-in user on a narrow viewport selects Job search
- **THEN** the app navigates to `/my/profile`

#### Scenario: Desktop menu excludes account items

- **WHEN** any user opens the menu on a wide viewport
- **THEN** it shows only the nav links and the theme toggle, with no account
  items, no Submit a job, no Moderation, and no Sign in/Log out action

#### Scenario: Moderator sees moderation on mobile

- **WHEN** a signed-in moderator opens the menu on a narrow viewport
- **THEN** the menu additionally shows the Moderation item

#### Scenario: Open link sits next to About

- **WHEN** any user opens the menu, on either viewport
- **THEN** the nav links include an Open item (targeting `/open`) immediately
  next to the About item

#### Scenario: Selecting an item closes the menu

- **WHEN** the menu is open and the user selects any link or action
- **THEN** the menu closes

### Requirement: Desktop profile menu

On a wide viewport, a signed-in user's profile icon SHALL act as a second,
independent menu trigger, opening a dropdown scoped to that user's own
account: the workspace switcher — Job search (targeting `/my/profile`), Mentor (beta
testers only, targeting `/my/mentorship/bookings`), Referrer (targeting
`/my/referrals/offers`) and Employer (targeting `/my/employer`),
each with a one-line description and the current workspace marked — then Settings
(targeting `/my/account`), then Submit a job, then (moderators only) Moderation, then Log
out. It SHALL NOT repeat the individual account sections. Opening the profile menu SHALL
close the consolidated menu and the notification bell dropdown if either is open, and
opening either of those SHALL close the profile menu. The profile menu SHALL close after an
item is selected, on `Escape`, on outside click, and on navigation. For a signed-out user on
a wide viewport the profile icon SHALL remain a direct Sign in action (no dropdown).

#### Scenario: Profile menu lists the account items

- **WHEN** a signed-in non-moderator, non-beta user opens the profile menu on a wide
  viewport
- **THEN** it shows Job search, Referrer, Employer, Settings, Submit a job, and Log out, but
  not Mentor and not Moderation

#### Scenario: The current workspace is marked

- **WHEN** a signed-in user on `/my/employer` opens the profile menu
- **THEN** Employer is marked as the current workspace and Job search is not

#### Scenario: Profile item targets the single profile

- **WHEN** a signed-in user selects Job search in the profile menu
- **THEN** the app navigates to `/my/profile`

#### Scenario: Moderator sees moderation in the profile menu

- **WHEN** a signed-in moderator opens the profile menu on a wide viewport
- **THEN** the profile menu additionally shows the Moderation item

#### Scenario: Log out is immediately visible

- **WHEN** a signed-in user opens the profile menu on a wide viewport
- **THEN** the Log out action is visible in the panel without needing to
  scroll

#### Scenario: Opening the profile menu closes the consolidated menu

- **WHEN** the consolidated menu is open on a wide viewport and the user opens
  the profile menu
- **THEN** the consolidated menu closes and the profile menu opens

#### Scenario: Opening the consolidated menu closes the profile menu

- **WHEN** the profile menu is open on a wide viewport and the user opens the
  consolidated menu
- **THEN** the profile menu closes and the consolidated menu opens

#### Scenario: Signed-out profile icon has no dropdown

- **WHEN** a signed-out user is on a wide viewport
- **THEN** the profile icon position shows a direct Sign in action and
  activating it does not show a dropdown panel

### Requirement: Active nav indication

A nav link in the menu SHALL be marked active when the current path equals its
target or is a sub-route of it, so the menu reflects where the user is.

#### Scenario: Current section is highlighted

- **WHEN** the user is on `/jobs` or `/jobs/:slug` and opens the menu
- **THEN** the Jobs link is shown as active

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

### Requirement: Paying-tier badge on the desktop profile icon

On viewports where the standalone signed-in profile icon is shown (desktop, `≥640px`), the
system SHALL display a small "PRO" or "ULTRA" badge on that icon when the signed-in
account's tier is `pro` or `ultra` respectively, and SHALL show no badge when the tier is
`free`.

#### Scenario: Pro subscriber sees a PRO badge

- **WHEN** a signed-in account whose tier is `pro` views any page on a desktop viewport
- **THEN** the header's profile icon shows a "PRO" badge

#### Scenario: Ultra subscriber sees an ULTRA badge

- **WHEN** a signed-in account whose tier is `ultra` views any page on a desktop viewport
- **THEN** the header's profile icon shows an "ULTRA" badge

#### Scenario: Free account sees no badge

- **WHEN** a signed-in account whose tier is `free` views any page on a desktop viewport
- **THEN** the header's profile icon shows no tier badge

#### Scenario: Signed-out visitor

- **WHEN** a signed-out visitor views any page on a desktop viewport
- **THEN** the header shows the sign-in control with no tier badge

