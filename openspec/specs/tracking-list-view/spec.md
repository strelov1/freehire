# tracking-list-view Specification

## Purpose
Reading the tracked applications as a list rather than a board, and finding one
by employer or role. Introduced by the tracker-drag-list-search change.

## Requirements
### Requirement: The tracked applications are readable as a list

The tracking section SHALL offer a list view of the caller's applications at
`/my/tracking/list`, presented as a third tab beside Board and Pipeline. The view
SHALL be its own URL so it is linkable, bookmarkable and survives a reload, and
SHALL be fed by the same server load the board uses rather than a second fetch.

The list SHALL show one row per application, ordered by last activity, newest
first. A row SHALL carry the employer, the role, the current stage as a control
that changes it, the days silent when the application is silent, and the count of
linked mail when there is any. Opening a row SHALL open the same application
panel the board opens.

The list SHALL show the same applications the board shows — those with an
application state — and SHALL NOT show a saved-only job, which is not an
application and lives in Activity.

#### Scenario: The list is its own URL

- **WHEN** a signed-in user opens `/my/tracking/list`
- **THEN** the list view renders with the List tab selected, and a reload returns
  to the same view

#### Scenario: A row opens the application

- **WHEN** the user selects a row in the list
- **THEN** the same application panel the board opens is shown for that row

#### Scenario: Ordered by last activity

- **WHEN** the list renders two applications, one whose last activity is more
  recent than the other's
- **THEN** the more recent one is listed first

#### Scenario: A saved-only job is not listed

- **WHEN** the caller has a job saved but never applied to
- **THEN** it does not appear in the list

### Requirement: Applications are searchable by employer and role

Both the board and the list SHALL offer one search field that narrows the shown
applications to those whose employer or role matches the query. The query SHALL
be synchronised to the `q` search parameter so a search is linkable and survives
a reload.

Matching SHALL be case-insensitive and SHALL apply to the rows already loaded —
the tracking listing is bounded, and a request per keystroke buys nothing.

An application whose posting the catalogue no longer holds SHALL be searchable on
the employer and role recorded on the application itself, since it has no posting
to read them from.

#### Scenario: Narrowing by employer

- **WHEN** the user types an employer's name into the search field
- **THEN** only applications to that employer are shown, on whichever view is open

#### Scenario: Narrowing by role

- **WHEN** the user types words from a role title
- **THEN** only applications whose role matches are shown

#### Scenario: The search is in the URL

- **WHEN** the user has narrowed the view and reloads the page
- **THEN** the same query is applied and the same applications are shown

#### Scenario: A posting-less application is searchable

- **WHEN** the caller searches for the employer of an application whose posting
  was removed from the catalogue
- **THEN** that application is shown

#### Scenario: Clearing the search restores every application

- **WHEN** the user clears the search field
- **THEN** every application is shown again and `q` leaves the URL
