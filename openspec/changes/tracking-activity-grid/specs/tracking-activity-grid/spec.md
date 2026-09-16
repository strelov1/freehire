## ADDED Requirements

### Requirement: The year of effort is readable as a grid

The tracking section SHALL offer an activity view of the caller's own job-search actions at
`/my/tracking/activity`, presented as a tab beside Board, List, Pipeline and Calendar. The view
SHALL be its own URL so it is linkable, bookmarkable and survives a reload.

The view SHALL draw roughly the last year as a grid of one square per day, seven rows deep and
ordered by week, oldest week at the left. A square SHALL be shaded by how many counting actions
fell on that day, across a fixed number of levels, with zero drawn as an empty level rather than
omitted — a day with nothing on it is part of the answer.

How long the window is SHALL be decided by what the endpoint will answer, not chosen: the
timeline refuses a span over `apptimeline.MaxRangeDays` outright rather than trimming it, the
request carries a day of margin at each end, and the cap is an ABSOLUTE duration while a
calendar day is 25 hours when the clocks go back. The window SHALL therefore leave slack for a
span containing two autumn transitions, and SHALL be verified across a whole year of start
dates rather than at one — a single date checked is a date that passes.

The grid SHALL carry the labels that make it readable without explanation: the month a week
column belongs to, the weekdays, and a legend running from less to more. A month's label SHALL
sit on the column that week ENDS in, because a column straddling the 1st sits mostly in the new
month and labelling it with the old one puts the caption a column left of what it names. The
weekday rail MAY label alternate rows only, and SHALL be hidden from assistive technology,
because every square already announces its own full date.

#### Scenario: The activity view is its own URL

- **WHEN** a signed-in user opens `/my/tracking/activity`
- **THEN** the grid renders with the Activity tab selected, and a reload returns to the same view

#### Scenario: A day with no actions is still drawn

- **WHEN** the caller did nothing on a day inside the window
- **THEN** that day appears as an empty square at the lowest level, not as a gap in the grid

#### Scenario: The window is the last year, ending today

- **WHEN** the grid is drawn
- **THEN** its last square is the reader's today and its first is no more than a year earlier

#### Scenario: The request fits the cap on every day of the year

- **WHEN** the span to fetch is worked out for any date in any timezone, including one whose
  window contains two autumn clock changes
- **THEN** its absolute duration is at most `apptimeline.MaxRangeDays`, so the endpoint answers
  rather than refusing and leaving every reader on that date with an error

### Requirement: A square counts only what the candidate did

A day's count SHALL include only the ledger events the CANDIDATE is responsible for: an event of
kind `applied` or `follow_up_sent`, and an event of kind `stage_set` whose source is the
candidate, the assistant, or auto-apply.

It SHALL exclude an event of kind `employer_reply`, and SHALL exclude a `stage_set` whose source
is the platform itself. Those record somebody else's action — an employer writing, or the
platform noticing a listing closed — and shading a square for one would credit the candidate with
work they did not do.

An event of a kind this build does not recognise SHALL NOT count. A counting rule that admits the
unknown would silently start rewarding whatever is added to the ledger next.

#### Scenario: Applying counts

- **WHEN** the caller applied to a job on a day
- **THEN** that day's count includes it, whether the application was submitted by the caller, by
  the assistant, or by auto-apply on their behalf

#### Scenario: An employer's reply does not count

- **WHEN** the only event on a day is an employer's reply
- **THEN** that day counts zero and is drawn as an empty square

#### Scenario: A stage the platform set does not count

- **WHEN** the only event on a day is a `stage_set` whose source is the platform
- **THEN** that day counts zero

#### Scenario: A stage the caller set counts

- **WHEN** the caller moved an application to another stage on a day
- **THEN** that day's count includes it

#### Scenario: An unrecognised kind does not count

- **WHEN** the ledger serves an event of a kind this build does not know
- **THEN** it is not counted toward any day

### Requirement: A day is decided by the reader's own clock

The day a counting action falls on SHALL be decided in the reader's local timezone, not the
server's. The server SHALL serve the moment an event occurred and SHALL NOT group events into
days.

`occurred_at` is an absolute moment. An application submitted at 23:40 UTC belongs to the next day
for a reader in Tokyo and to the same day for one in London, and only the browser knows which. A
day boundary crossed the wrong way moves a square, and a square moved across the boundary of a
streak breaks or invents one.

Day arithmetic SHALL be done by calendar date rather than by adding fixed millisecond spans, so a
daylight-saving boundary inside the window does not shift every later square.

#### Scenario: A late-evening action is filed by the reader's date

- **WHEN** an application was submitted at an instant that is one date in UTC and the next in the
  reader's zone
- **THEN** the square shaded is the one for the reader's date

#### Scenario: A clock change does not shift the grid

- **WHEN** the window spans a daylight-saving transition
- **THEN** every square after it still stands for its own calendar date

### Requirement: The streak is what the view is for

The view SHALL report three figures beneath the grid: the total counting actions in the window,
the current streak, and the longest streak inside the window.

The current streak SHALL be the number of consecutive days ending today on which the caller did
at least one counting action. A day on which they did nothing SHALL end the streak.

Today having no actions yet SHALL NOT be read as a broken streak while the day is still running:
a streak that ran up to yesterday SHALL be reported as intact, so the figure does not reset every
midnight and tell somebody they lost a run they are still in the middle of.

The longest streak SHALL be the longest such run of consecutive active days anywhere in the
window.

#### Scenario: Consecutive active days make a streak

- **WHEN** the caller acted on each of the last five days including today
- **THEN** the current streak reads five

#### Scenario: Today being empty does not end a live streak

- **WHEN** the caller acted on each of the five days ending yesterday and has not acted today
- **THEN** the current streak still reads five

#### Scenario: A gap ends the streak

- **WHEN** the caller acted today and last acted three days ago
- **THEN** the current streak reads one

#### Scenario: A window with no actions reports zero

- **WHEN** the caller has no counting actions in the window
- **THEN** all three figures read zero and the view says so in words rather than showing an
  unexplained empty grid

### Requirement: Selecting a day shows what happened on it, without a request

Selecting a square SHALL open a panel listing that day's events, captioned with the same
vocabulary the calendar and the application panel use, so one event is never worded two ways on
two screens. Selecting the same square again SHALL close the panel.

The panel SHALL be filled from data already in hand and SHALL NOT issue a request of its own: the
whole window is fetched once for the grid.

The panel SHALL list the day's events INCLUDING the ones that do not count toward the square. What
a square measures is effort; what a day held is history, and a day whose only event was an
employer's reply must still be readable rather than appearing to hold nothing.

#### Scenario: A day's events are listed on selection

- **WHEN** the caller selects a square holding events
- **THEN** the panel lists them, oldest first, each captioned by its kind

#### Scenario: The panel makes no request

- **WHEN** the caller selects any square
- **THEN** no further call to the timeline endpoint is made

#### Scenario: A non-counting event is still visible

- **WHEN** the caller selects a day whose only event is an employer's reply
- **THEN** the square is empty but the panel lists that reply

#### Scenario: Selecting the open day closes it

- **WHEN** the caller selects the square already selected
- **THEN** the panel closes

### Requirement: The view is served by the published timeline and adds no endpoint

The view SHALL read the caller's events through the existing `GET /me/timeline`, asking for the
window in one request. It SHALL NOT introduce an endpoint, a table, a migration, or a stored
aggregate.

The page SHALL be served with its data rather than fetching on mount, in the shape the tracking
section's other views use. A failed fetch on the server SHALL fall back to a client fetch and a
friendly error state rather than failing the page.

The server's day is not necessarily the reader's, so the window fetched on the server SHALL carry
enough margin that no square the reader draws falls outside it.

#### Scenario: The grid arrives with the page

- **WHEN** the server load succeeds
- **THEN** the grid renders on first paint without a client fetch

#### Scenario: A server-side failure degrades rather than 500s

- **WHEN** the server load fails
- **THEN** the page still renders and the view fetches the window itself, showing a friendly
  error if that also fails
