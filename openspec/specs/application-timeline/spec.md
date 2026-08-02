# application-timeline Specification

## Purpose
Reading the application-event ledger as a dated series for one caller: the range read
behind `GET /me/timeline`, what each event carries to the wire, how the provenance of a
date is reported, and what happens to an event whose message was deleted. Introduced by
the tracking-calendar-view change, as the ledger's first reader that asks *when*.

## Requirements

### Requirement: The ledger is readable as a dated range

The application-event ledger SHALL be readable as a series of one caller's events over a date
range, through `GET /me/timeline?from=&to=` and the service behind it. The read SHALL return
only the caller's own events and SHALL exclude retracted ones, matching every aggregate that
already reads the ledger.

The read SHALL live in a service package rather than in the Fiber handler. The mail stack
already pins this rule and the ledger spec repeats it: the in-app assistant calls services
directly with the session owner's id and issues no HTTP request, so a rule enforced in a
handler is a rule the in-process reader never meets.

The range SHALL be bounded by the caller's `from` and `to`, and the response SHALL report the
range it answered for, so a reader can tell a quiet month from a request it mis-addressed.

#### Scenario: The range is the caller's own

- **WHEN** two users each have events on the same day and one requests that day's range
- **THEN** only the requesting user's events are returned

#### Scenario: A retracted event is not served

- **WHEN** an email is re-linked from company A to company B and the range covers its date
- **THEN** A's retracted event is absent from the series and B's replacement is present at the
  same `occurred_at`

#### Scenario: An empty range answers with an empty series

- **WHEN** the caller requests a range in which nothing happened
- **THEN** the response is an empty series reporting the range requested, not an error

### Requirement: The server reports whether an event was observed

Each event SHALL carry a verdict saying whether its date was observed or recorded by hand, and
that verdict SHALL be computed on the server from `appevent.TrustedForDayMath`. A reader SHALL
NOT derive it from the source name itself.

Only the mail sources carry a date somebody other than the candidate set. A stage the candidate
switched records when they got around to updating their board, not when the employer moved
them. Surfacing both without the distinction would draw the same confidence under each. Placing
the rule in the reader would fork it: the vocabulary has grown before, and a second copy would
still call a newly added source observed while the first had already refused it.

#### Scenario: A mail-derived event reports as observed

- **WHEN** an `employer_reply` recorded from Gmail, hosted or external mail is served
- **THEN** it reports as observed

#### Scenario: A hand-recorded event reports as unobserved

- **WHEN** a `stage_set` the candidate made from the board is served
- **THEN** it reports as not observed, whichever stage it moved to

#### Scenario: A new source cannot slip through as observed

- **WHEN** a source is added to the `appevent` vocabulary
- **THEN** the served verdict for it still equals `appevent.TrustedForDayMath` for that source,
  and the test asserting this over the whole vocabulary fails until the source has a verdict

### Requirement: An event outlives its message, its subject does not

An event SHALL be served whether or not the message that produced it still exists to the
reader. Where the message is present and not deleted, the event MAY carry that message's
subject and its identifier. Where the message was deleted, the event SHALL be served without
them.

The ledger already holds this position for aggregates: deleting a message hides content, it
does not un-happen the reply. A series that dropped the event would repeat the distortion the
ledger was written to remove; one that showed the subject anyway would serve content the reader
asked to be rid of.

No content beyond the subject SHALL be served. Reading a message body through
`GET /me/emails/:id` marks it read, and `read_at` means a human saw it; a series assembled from
that endpoint would silently zero its owner's unread count.

#### Scenario: A deleted message leaves the event standing

- **WHEN** the range covers an `employer_reply` whose email was deleted
- **THEN** the event is served with its date, employer and signal, carrying neither subject nor
  message identifier

#### Scenario: A live message lends its subject

- **WHEN** the range covers an `employer_reply` whose email is present
- **THEN** the event carries that email's subject and identifier, and the email is not marked
  read

### Requirement: The wire shape does not enumerate the kind vocabulary

An event's `kind` and `signal` SHALL be served as values from the `appevent` and
`mailclassify` vocabularies rather than as a fixed set the response format enumerates. A reader
SHALL render a kind it does not recognise rather than failing on it.

`application_events` separates `kind` from `signal` precisely so that a growing vocabulary is
not a change to a table that must not change. A response format, or a reader, that hard-codes
today's four kinds would put that cost back — one addition would then be a change to the
schema, the reader and their tests at once.

#### Scenario: An unrecognised kind still renders

- **WHEN** the series carries an event whose kind the reader does not know
- **THEN** the event is shown with its date, employer and role rather than dropped or errored

### Requirement: The ledger is readable for one application

The system SHALL read a single application's live events, newest first, scoped to the caller and
bounded by a limit. The read SHALL exclude retracted events, join the message for its subject
under the same conditions the range read uses, and take the employer from the event's own
denormalized slug rather than through the posting — the same rules, so the two reads cannot
disagree about what an event is or which employer it belongs to.

The rule SHALL live in `internal/apptimeline` beside the range read rather than in an HTTP
handler: the in-app assistant calls services directly with the session owner's id and issues no
HTTP request, and "what happened to this application" is a question it should be able to ask.

#### Scenario: Newest first

- **WHEN** an application has an apply, a reply and a stage change
- **THEN** they are returned newest first, with a stable order between events sharing a timestamp

#### Scenario: Retracted events are absent

- **WHEN** an event has been retracted, as a re-linked message's employer_reply is
- **THEN** it does not appear in the application's history

#### Scenario: Scoped to the caller

- **WHEN** two users have events against the same job
- **THEN** each reads only their own

#### Scenario: An event outlives its message

- **WHEN** the message an event was derived from has been deleted
- **THEN** the event is still returned, carrying no subject — deletion hides content, it does not
  un-happen the reply

#### Scenario: The read is bounded

- **WHEN** an application has accrued more events than the limit
- **THEN** the newest are returned, up to the limit

### Requirement: The application panel renders its history

The application panel SHALL show the application's events as its history: newest first, each with
its time and what happened, rendered from the shared event vocabulary so a row reads the same
there as on the calendar. The events SHALL arrive on the existing single-application read rather
than on a request of their own.

The panel SHALL NOT show an engagement funnel in place of a history. Ordering `viewed`, `saved`
and `applied` by depth put the newest fact on the left and the oldest on the right, which reads
as a sequence and is not one.

`viewed` and `saved` SHALL be absent from the history. They are marks on a posting rather than
events of an application, and `viewed_at` is refreshed on every view, so presenting it as the
first step would state a first view while holding the date of the most recent one.

#### Scenario: A settled application shows how it settled

- **WHEN** an application was applied to, chased, moved to interview and then rejected by mail
- **THEN** the panel lists all four, newest first

#### Scenario: An application with no events

- **WHEN** an application has been saved but never applied to, so the ledger holds nothing
- **THEN** the panel shows no history section rather than an empty frame

#### Scenario: One vocabulary with the calendar

- **WHEN** the same event is shown in the panel and on the calendar
- **THEN** it carries the same label in both, read from one definition

#### Scenario: A kind without a label fails the build

- **WHEN** an event kind is added to the Go vocabulary and not given a label
- **THEN** the frontend check fails rather than rendering a blank row
