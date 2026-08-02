## ADDED Requirements

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
