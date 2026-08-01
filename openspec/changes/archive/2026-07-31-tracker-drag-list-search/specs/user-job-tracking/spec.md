## ADDED Requirements

### Requirement: Writing an application by the row the listing served

The system SHALL let an authenticated user set an application's `stage` and/or
`notes` via `PATCH /api/v1/me/applications/:id`, drop it from the board via
`DELETE /api/v1/me/applications/:id`, and clear its progress while keeping its
saved mark via `DELETE /api/v1/me/applications/:id/stage`. The body, the stage
vocabulary, the partial-update rule and the rejection of an unknown stage SHALL be
those the slug-addressed track endpoint already applies.

`:id` SHALL be the row identifier exactly as `GET /api/v1/me/tracking` served it.
That identifier has two forms — an application whose posting the catalogue no
longer holds is named by the application, every other row by its posting's slug —
and the endpoint SHALL accept both, because the interface can only send back what
the listing gave it.

An identifier that names nothing the caller owns SHALL be answered `404` with the
body a missing row produces, whatever its form. "Not an identifier" and "not
yours" MUST be one answer.

The slug-addressed routes SHALL remain registered and unchanged: they are how the
`freehire-cli` and `freehire-mcp` address a posting, and those clients hold no row
identifiers.

#### Scenario: Moving an application whose posting was removed

- **WHEN** the caller sends `PATCH /api/v1/me/applications/:id` with
  `{"stage":"interview"}` for an application the catalogue no longer holds a
  posting for
- **THEN** the stage is recorded and the updated record is returned

#### Scenario: Moving an ordinary application

- **WHEN** the caller sends the same request for a row the listing named by its
  posting's slug
- **THEN** the stage is recorded exactly as the slug-addressed endpoint records it

#### Scenario: Dropping an application from the board

- **WHEN** the caller sends `DELETE /api/v1/me/applications/:id`
- **THEN** the application leaves the board and the row is no longer listed there

#### Scenario: Clearing progress keeps the saved mark

- **WHEN** the caller sends `DELETE /api/v1/me/applications/:id/stage` for a saved
  application
- **THEN** the application leaves the board and the job remains saved

#### Scenario: An identifier that names nothing

- **WHEN** the caller sends any of these requests with an identifier that is
  malformed, or that names a row belonging to somebody else
- **THEN** the system responds `404` with the same body in both cases, and changes
  nothing

#### Scenario: The slug-addressed routes still answer

- **WHEN** a client sends `PATCH /api/v1/jobs/:slug/track`
- **THEN** it behaves exactly as before this change

## MODIFIED Requirements

### Requirement: SPA shows and edits application stage and notes

The web SPA's tracking board SHALL, for a signed-in user, show each application's
`stage` as a humanized badge when set, let the user change the stage by dragging
the card between the board's columns or from a control in the opened application
offering the stage vocabulary, and let the user edit `notes` in the opened
application. A signed-out user SHALL see no such controls.

A board card SHALL carry no controls. It is dragged and it is opened, and it
SHALL be draggable from anywhere on it — a card that mounts an interactive
element over its surface cannot be picked up, because the drag library refuses a
gesture that begins on one. Everything the candidate can do to an application
SHALL be offered in the opened application, which has the room for it.

The card SHALL keep its indicators — the stage badge, the silence marker, the
count of linked mail, the mark that it has notes — none of which is a control.

The opened application SHALL offer, beside its stage and notes: a rehearsal, a
follow-up draft, the fit analysis, and CV tailoring. Those that need the posting
SHALL be absent, not disabled, for an application whose posting the catalogue no
longer holds.

#### Scenario: Change a stage by dragging

- **WHEN** a signed-in user drags a card from anywhere on its surface into another
  column
- **THEN** the drag begins, the card lands in that column, and the new stage is
  persisted

#### Scenario: A card carries no controls

- **WHEN** the board renders an application card
- **THEN** the card presents no buttons, and clicking anywhere on it opens the
  application

#### Scenario: Change a stage from the opened application

- **WHEN** a signed-in user selects a new stage in the opened application
- **THEN** the SPA persists it and reflects the new stage on the board

#### Scenario: Edit notes

- **WHEN** a signed-in user edits an application's notes and the field loses focus
- **THEN** the SPA persists the notes

#### Scenario: The actions are offered where there is room

- **WHEN** a signed-in user opens an application whose posting is still listed
- **THEN** the panel offers a rehearsal, the fit analysis and CV tailoring, and
  offers a follow-up draft when the application is owed one

#### Scenario: A posting-less application offers only what it can

- **WHEN** the user opens an application whose posting the catalogue no longer
  holds
- **THEN** the actions needing the posting are absent, and the stage, the notes
  and the follow-up draft still work
