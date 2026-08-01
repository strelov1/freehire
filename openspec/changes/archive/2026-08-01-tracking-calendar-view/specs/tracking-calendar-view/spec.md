## ADDED Requirements

### Requirement: The search is readable as a calendar

The tracking section SHALL offer a calendar view of the caller's application events at
`/my/tracking/calendar`, presented as a tab beside Board, List and Pipeline. The view SHALL be
its own URL so it is linkable, bookmarkable and survives a reload.

The view SHALL show one month at a time, defaulting to the current month, with controls to move
to the previous and next month. A day SHALL show what happened on it: the events of that day,
distinguished by kind, with a count where there are more than the cell can hold.

The calendar SHALL show application events — what happened to applications — and SHALL NOT show
catalogue marks such as a viewed or saved job, which are not events in the ledger and live in
Activity.

#### Scenario: The calendar is its own URL

- **WHEN** a signed-in user opens `/my/tracking/calendar`
- **THEN** the calendar renders with the Calendar tab selected, and a reload returns to the same
  view

#### Scenario: Moving between months

- **WHEN** the user moves to the previous month
- **THEN** that month's grid is shown with its own events

#### Scenario: A crowded day reports its count

- **WHEN** a day holds more events than its cell shows
- **THEN** the cell shows the events it can and reports how many remain

#### Scenario: A saved job is not an event

- **WHEN** the caller saved a job without applying
- **THEN** no mark for it appears on any day

### Requirement: A day is decided by the reader's own clock

The day an event falls on SHALL be decided in the reader's local timezone, not the server's.
The server SHALL serve the moment an event occurred and SHALL NOT group events into days.

`occurred_at` is an absolute moment. A reply that arrived at 23:40 UTC belongs to the next day
for a reader in Warsaw and to the same day for one in London, and only the reader's browser
knows which. The rendered range SHALL therefore be requested with a margin of one day on each
side, or the first and last cells of a month would be short of events that belong in them.

Because the server render cannot know the reader's timezone, the grid SHALL be arranged in the
browser; the server load SHALL fetch data only.

#### Scenario: A late-evening event lands on the reader's day

- **WHEN** an event occurred at 23:40 UTC and the reader's clock is an hour ahead
- **THEN** it appears on the following day's cell

#### Scenario: The edges of the month are complete

- **WHEN** the caller opens a month whose first day holds an event that occurred late on the
  previous day in UTC
- **THEN** that event appears in the first day's cell

### Requirement: A day opens without a further request

Selecting a day SHALL open a panel beneath the grid listing that day's events, and that panel
SHALL be assembled from the data already fetched for the range. Selecting a day SHALL NOT issue
a request.

The rule is a guard as much as a performance choice. The panel names each event's employer,
role and — for a mail-derived event — the subject of the message; fetching that per selection
would reach for the message endpoint, which marks mail read. A panel that cannot request cannot
make that mistake, and browsing a month cannot silently empty the caller's unread count.

Each event in the panel SHALL offer the application it belongs to, and, where the event came
from a message that still exists, that message in the inbox.

#### Scenario: Selecting a day issues no request

- **WHEN** the user selects a day in the rendered month
- **THEN** the panel lists that day's events with no network request made

#### Scenario: A mail-derived event reaches its message

- **WHEN** the panel lists an event whose message still exists
- **THEN** it offers both the application and that message in the inbox

#### Scenario: An event whose message was deleted still reaches its application

- **WHEN** the panel lists an event whose message was deleted
- **THEN** it offers the application, shows no subject, and offers no message

### Requirement: An event says whether it was observed or recorded

The calendar SHALL distinguish an event whose date was observed from one the candidate recorded
by hand, in the grid and in the day panel, using the verdict the server serves rather than one
derived in the browser.

A calendar makes the distinction physical: a mark sits on a particular cell, and a hand-recorded
stage change drawn identically to an employer's reply claims the same authority for the day the
candidate updated their board as for the day somebody answered.

#### Scenario: A hand-recorded stage change is marked as such

- **WHEN** the day panel lists a stage change the candidate made from the board
- **THEN** it is presented as recorded by the candidate rather than as an observed moment

### Requirement: The calendar is legible on a narrow screen and honest when empty

On a viewport too narrow for seven columns the view SHALL present the month as a list of the
days that hold events rather than a grid.

Where the month on screen holds nothing, the view SHALL name that month, say that nothing was
recorded in it, and offer the tracking board — rather than render a bare empty grid, which
reads as a fault in the calendar rather than as a quiet month.

The message SHALL be about the month and not about the account. The view fetches one month at
a time and cannot tell an empty history from a quiet August without a second read of the whole
ledger; a message claiming the stronger thing would be a guess.

#### Scenario: A narrow viewport lists days

- **WHEN** the calendar renders on a viewport too narrow for a seven-column grid
- **THEN** the month is presented as a list of days holding events

#### Scenario: An empty month explains itself

- **WHEN** the caller opens a month in which nothing was recorded
- **THEN** the view names that month, says nothing was recorded in it, and offers the tracking
  board, while the month controls stay usable so another month can be looked at
