# application-silence-signal Specification

## Purpose
TBD - created by archiving change application-ghosting-signal. Update Purpose after archive.
## Requirements
### Requirement: Deriving an application's last activity

The system SHALL derive, for each tracked application, a last-activity timestamp
as the later of the application's `applied_at` and the most recent `received_at`
among the emails linked to that application for that user. Only linked mail
counts: an email carrying a suggestion that the user has not confirmed is not
activity. Soft-deleted emails are excluded. An application with no linked mail
takes its `applied_at` as its last activity, so the derived value is never null
for an application.

#### Scenario: Application with no linked mail

- **WHEN** an application has `applied_at` set and no linked emails
- **THEN** its last activity equals `applied_at`

#### Scenario: Linked mail moves the last activity forward

- **WHEN** an application has a linked email received after `applied_at`
- **THEN** its last activity equals that email's `received_at`

#### Scenario: Mail linked to another application is ignored

- **WHEN** a user has a linked email belonging to a different application
- **THEN** that email does not affect this application's last activity

#### Scenario: Unconfirmed suggestion is not activity

- **WHEN** an email carries `suggested_job_id` for the application but has no
  `job_id`
- **THEN** the application's last activity is unchanged by that email

#### Scenario: Deleted mail is ignored

- **WHEN** a linked email has been soft-deleted by the user
- **THEN** it does not contribute to the application's last activity

### Requirement: Stage-aware silence thresholds

The system SHALL judge an application's silence against a threshold determined by
its current stage, growing stricter as the application advances: `applied` 21
days, `screening` 18 days, `responded` 15 days, `interview` 12 days, `offer` 5
days. An application whose stage is unset is judged as `applied`. The days-silent
value is the whole days elapsed between the application's last activity and now.

Each value SHALL carry its provenance in the source, because a table of five
specific numbers reads as measurement whether or not it is one, and only two of
these are:

| Stage | Days | Source |
|---|---|---|
| `applied` | 21 | measured — 92 observed applications, marks 16% of them |
| `screening` | 18 | interpolated between the two measured anchors |
| `responded` | 15 | interpolated between the two measured anchors |
| `interview` | 12 | measured — 6 observed applications, marks 3; raised from 7, which marked 5 of 6 |
| `offer` | 5 | judgement, from a job seeker's experience — no application in the sample has reached this stage. Exactly one message was ever classified an offer, and it is genuine but from a job search three years earlier, in the archive cohort this work deliberately left unlabelled. It informs nothing |

The interpolated pair steps evenly by three days between 21 and 12 rather than
taking distinct-looking values, so the shape of the ladder shows at a glance
which rungs were derived. This mirrors the dictionaries rule elsewhere in the
project — never guess, emit nothing for an unknown — as closely as a threshold
can, since a stage with no threshold cannot be judged at all.

The thresholds are calendar days, which sets a floor under the strictest of them.
Five is the shortest span that always contains at least two working days; three
can be consumed entirely by a weekend, so an offer arriving on a Friday evening
would be reported as ignored before anyone had a working day to answer. Anything
below five needs business-day arithmetic — a separate concern, with its own
calendar, holidays and employer time zone.

#### Scenario: Below the threshold

- **WHEN** an application at stage `applied` has been silent for 10 days
- **THEN** its silence state is `active` and its days-silent is 10

#### Scenario: Past the threshold

- **WHEN** an application at stage `applied` has been silent for 22 days
- **THEN** its silence state is `silent`

#### Scenario: The same silence is judged differently by stage

- **WHEN** two applications have both been silent for 13 days, one at stage
  `applied` and one at stage `interview`
- **THEN** the `applied` one is `active` and the `interview` one is `silent`

#### Scenario: Unset stage is judged as applied

- **WHEN** an application has `applied_at` set and a null stage
- **THEN** it is judged against the `applied` threshold of 21 days

### Requirement: Terminal applications never accrue silence

The system SHALL report no silence state and no days-silent for an application in
a terminal stage — `rejected`, `accepted`, or `withdrawn`. A settled outcome is
not awaiting a reply, so counting its silence would manufacture an alarm about a
closed matter.

#### Scenario: Rejected application

- **WHEN** an application at stage `rejected` has had no activity for a year
- **THEN** its silence state and days-silent are both null

#### Scenario: Accepted application

- **WHEN** an application at stage `accepted` has had no activity for 90 days
- **THEN** its silence state and days-silent are both null

### Requirement: An unconfirmed suggestion outranks a silence claim

The system SHALL report the silence state `unconfirmed`, rather than `silent`,
for an application that has at least one email suggesting it that the user has
not yet confirmed or rejected. Mail the matcher believes belongs to this
application contradicts the claim that nobody replied, so the surface asks for a
confirmation instead of asserting a silence that may be false.

#### Scenario: Past the threshold with a pending suggestion

- **WHEN** an application is past its stage's silence threshold and has an
  unconfirmed email suggesting it
- **THEN** its silence state is `unconfirmed`, not `silent`

#### Scenario: Below the threshold with a pending suggestion

- **WHEN** an application is below its silence threshold and has an unconfirmed
  email suggesting it
- **THEN** its silence state is `active` — a pending suggestion only suppresses a
  silence claim, it does not raise one

#### Scenario: Confirming the suggestion resolves the state

- **WHEN** the user confirms the suggested link and the email's `received_at` is
  recent enough to clear the threshold
- **THEN** the application's silence state becomes `active` and its last activity
  is that email's `received_at`

#### Scenario: Rejecting the suggestion restores the silence claim

- **WHEN** the user rejects the suggestion, leaving the application with no
  pending suggestion and no new linked mail
- **THEN** the application's silence state becomes `silent`

### Requirement: Tracking board marks a silent application

The web SPA's tracking board SHALL mark each application card with its silence
state, showing the days silent for an application that is `silent` and an
invitation to confirm the pending mail for one that is `unconfirmed`. A card in
the `active` state and a card for a terminal application SHALL carry no silence
marker, so the marker means something when it appears.

A `silent` card SHALL additionally offer the candidate a follow-up draft, and — once a follow-up has
been recorded — SHALL say that the application was chased and when, without dropping or softening the
silence marker. "Chased, still nothing" and "nobody has done anything about it" are different
situations, and the card MUST NOT render them the same way.

#### Scenario: Silent card

- **WHEN** a signed-in user opens the tracking board with an application whose
  silence state is `silent` at 24 days
- **THEN** that card shows a silence marker reading 24 days

#### Scenario: Unconfirmed card

- **WHEN** a card's application has the silence state `unconfirmed`
- **THEN** the card invites the user to confirm the pending mail rather than
  reporting a silence

#### Scenario: Active and terminal cards are unmarked

- **WHEN** a card's application is `active` or in a terminal stage
- **THEN** the card shows no silence marker

#### Scenario: A silent card offers a draft

- **WHEN** a card's application is `silent`
- **THEN** the card offers a follow-up draft

#### Scenario: A chased card keeps its silence marker

- **WHEN** a card's application is `silent` and a follow-up has been recorded
- **THEN** the card still shows the silence marker, and additionally reports that it was chased

