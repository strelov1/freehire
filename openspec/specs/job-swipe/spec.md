# job-swipe Specification

## Purpose
TBD - created by archiving change add-job-swipe-mode. Update Purpose after archive.
## Requirements
### Requirement: Entering swipe mode from the jobs page

The system SHALL provide a swipe-mode entry point in the `/jobs` toolbar,
visible on all viewport sizes. Activating it SHALL navigate to `/jobs/swipe`
carrying the page's currently applied filters and free-text query as URL
parameters, in the same parameter shape the jobs list uses, so the deck reflects
exactly what the user was looking at.

#### Scenario: Launch swipe mode with active filters

- **WHEN** a user viewing `/jobs?q=go&work_mode=remote` activates the swipe-mode
  entry point
- **THEN** the browser navigates to `/jobs/swipe?q=go&work_mode=remote`
- **AND** the deck is built from jobs matching those same filters

#### Scenario: Launch swipe mode with no filters

- **WHEN** a user with no active filters activates the swipe-mode entry point
- **THEN** the browser navigates to `/jobs/swipe`
- **AND** the deck is built from the unfiltered catalogue

### Requirement: Swipe mode requires an authenticated user

Swipe mode's two actions (save and dismiss) are per-user, so the mode SHALL be
available only to a signed-in user. When an unauthenticated visitor opens
`/jobs/swipe`, the system SHALL present a sign-in prompt instead of a deck and
SHALL NOT call the deck endpoint.

#### Scenario: Anonymous visitor opens swipe mode

- **WHEN** a visitor with no session opens `/jobs/swipe`
- **THEN** the page shows a prompt to sign in
- **AND** no deck request is made

#### Scenario: Signed-in user opens swipe mode

- **WHEN** a signed-in user opens `/jobs/swipe`
- **THEN** the deck loads and the first card is shown

### Requirement: The deck endpoint mirrors search and excludes judged jobs

The system SHALL expose an authenticated endpoint `GET /api/v1/me/tracking/swipe`
that returns a batch of open jobs. It SHALL accept the same facet and free-text
query parameters as `/api/v1/jobs/search` and run the same Meilisearch query, so
ranking and filtering match the list. It SHALL exclude the caller's already
**saved** and **dismissed** jobs by applying a server-built `id NOT IN
(excluded)` filter. The response SHALL use the standard list envelope
(`{"data": [...], "meta": {...}}`) with each item in the shared `jobview` shape,
and SHALL support batching via `limit`/`offset` for prefetch.

#### Scenario: Deck excludes saved and dismissed jobs

- **WHEN** a signed-in user who has saved job A and dismissed job B requests
  `GET /api/v1/me/tracking/swipe` with filters that would otherwise match A and B
- **THEN** neither A nor B appears in the returned batch
- **AND** other matching jobs are returned in search order

#### Scenario: Deck honors filters and query

- **WHEN** the request carries `q` and facet parameters
- **THEN** the returned jobs match the same query and facets that
  `/api/v1/jobs/search` would apply

#### Scenario: Deck requires authentication

- **WHEN** a request to `GET /api/v1/me/tracking/swipe` carries neither a valid auth
  cookie nor a valid API key
- **THEN** the system responds `401` and returns no deck

#### Scenario: Deck pages via limit and offset

- **WHEN** the client requests a second batch with a larger `offset`
- **THEN** the system returns the next page of matching, non-excluded jobs
  without repeating jobs from the first batch

### Requirement: Card presentation

Each card SHALL present the job with its company logo, title, company name and
location, salary when available, and a set of facet chips (such as work mode,
skills, seniority, and domain), plus a short description excerpt. Card content is
drawn from the `jobview` fields already returned by the deck endpoint.

#### Scenario: Card renders job context

- **WHEN** a card is shown for a job that has a salary and facets
- **THEN** the card displays the logo, title, company, location, salary, facet
  chips, and a description excerpt

### Requirement: Save, dismiss, open, and undo actions

For the active card the system SHALL support four actions. **Save** SHALL record
the save interaction (`POST /jobs/:slug/save`) and advance to the next card.
**Dismiss** SHALL record the dismissed interaction (`POST /jobs/:slug/dismiss`)
and advance to the next card. **Open** SHALL open the job's detail view without
recording save or dismiss. **Undo** SHALL revert the immediately previous save or
dismiss (`DELETE /jobs/:slug/save` or `DELETE /jobs/:slug/dismiss`) and restore
that card as the active one. Undo SHALL be limited to the most recent action.

#### Scenario: Save advances the deck

- **WHEN** the user saves the active card
- **THEN** the job is recorded as saved and the next card becomes active

#### Scenario: Dismiss advances the deck

- **WHEN** the user dismisses the active card
- **THEN** the job is recorded as dismissed and the next card becomes active

#### Scenario: Undo restores the last card

- **WHEN** the user triggers undo right after dismissing a job
- **THEN** that job's dismissed mark is cleared
- **AND** its card is restored as the active card

#### Scenario: Open does not judge the job

- **WHEN** the user opens the active card's detail view
- **THEN** the job is neither saved nor dismissed and remains in the deck

### Requirement: Input across mobile and desktop

The system SHALL accept touch swipe gestures on the active card — swipe right to
save, swipe left to dismiss — committed past a movement threshold with an exit
animation. On desktop the same save and dismiss decisions SHALL be available via
on-screen ✗ (dismiss) and ♥ (save) buttons and via the `←` (dismiss) and `→`
(save) arrow keys; the open and undo actions SHALL also be keyboard-accessible.

#### Scenario: Swipe right on mobile saves

- **WHEN** the user drags the active card to the right past the threshold on a
  touch device
- **THEN** the card commits a save and animates off-screen to the right

#### Scenario: Arrow keys on desktop

- **WHEN** the user presses the `→` arrow key on desktop
- **THEN** the active card is saved and the next card becomes active

### Requirement: Deck exhaustion and prefetch

The system SHALL prefetch the next batch before the deck runs low so swiping
stays uninterrupted. When no further matching, non-excluded jobs remain, the
system SHALL show an empty state that explains the deck is exhausted and links
back to the `/jobs` list to widen filters.

#### Scenario: Prefetch keeps the deck full

- **WHEN** the number of remaining cards drops below the prefetch threshold and
  more matching jobs exist
- **THEN** the client fetches the next batch and appends it without a visible
  interruption

#### Scenario: Empty deck

- **WHEN** the deck has no remaining cards and no further jobs match
- **THEN** an empty state is shown with a link back to `/jobs`

