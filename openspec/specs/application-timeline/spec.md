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
