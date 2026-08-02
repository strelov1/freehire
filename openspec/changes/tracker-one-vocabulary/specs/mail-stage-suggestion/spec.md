## ADDED Requirements

### Requirement: A classified message says what it implies for the stage

The system SHALL show, for every classified message linked to an application, both its status
signal and the application stage that signal implies — or that it implies none. The
signal→stage mapping SHALL be exported from `internal/mailclassify` rather than kept private,
and SHALL be emitted into the generated frontend contracts beside the existing status-signal
vocabulary, so the reader and the classifier cannot disagree about what a signal means.

A signal that never advances a stage (a rejection, an information request, an unfinished
application, anything unrecognised) SHALL say so explicitly rather than render as an
unexplained label.

#### Scenario: An acknowledgement names the stage it implies

- **WHEN** a message classified `acknowledgement` is shown on its application
- **THEN** it renders its signal together with the stage `Applied` that the signal implies

#### Scenario: A rejection says it moves nothing

- **WHEN** a message classified `rejection` is shown on its application
- **THEN** it renders its signal together with a statement that the signal does not move the
  stage

#### Scenario: The mapping has one definition

- **WHEN** a signal is added to the classifier's vocabulary
- **THEN** the label and the implied stage reach the reader from the generated contracts,
  without a second table to edit

### Requirement: A disagreement between mail and stage is offered for resolution

The system SHALL offer a one-click stage change when the newest classified message linked to an
application implies a stage that differs from the application's current stage. The offer SHALL
name the message it came from and the stage it proposes.

The offer SHALL NOT change any stage by itself. Automatic advancement is unchanged by this
capability: a message still advances a stage only strictly forward, only from a deterministic
link, and never into or out of a terminal stage. Deciding that an application is rejected,
accepted or withdrawn remains the candidate's call, and this offer is how that rule becomes
visible instead of silent.

#### Scenario: A rejection arrives on an interviewing application

- **WHEN** the newest classified message on an application at stage `interview` is a rejection
- **THEN** the reader is offered a one-click change to `Rejected`, naming that message

#### Scenario: Accepting the offer records the change as the candidate's

- **WHEN** the candidate accepts the offer
- **THEN** the stage is set exactly as a manual stage change is, and the ledger records it with
  the candidate as its source, not the mail

#### Scenario: A message agreeing with the stage offers nothing

- **WHEN** the newest classified message implies the stage the application already carries
- **THEN** no offer is shown

#### Scenario: Unclassified mail offers nothing

- **WHEN** the newest linked message carries no classification, as every `external` message
  does by design
- **THEN** no offer is shown

#### Scenario: An application with no explicit stage already counts as applied

- **WHEN** an application has `applied_at` set, no explicit stage, and its newest classified
  message is an acknowledgement
- **THEN** no offer is shown, because an unset stage already reads as `applied` on the board
  and in the counts, and offering to move it there would prompt on the commonest application
  there is

### Requirement: An answered question is not asked again

The system SHALL suppress the offer once the candidate has set the stage after the message that
prompted it, determined by reading the application-event ledger for a `stage_set` later than
that message. The system SHALL NOT introduce a dismissal flag for this purpose: the ledger
already records the candidate's decisions, and a second store of the same fact would be a
second thing to keep true.

#### Scenario: A stage set after the message silences the offer

- **WHEN** the candidate sets any stage after the message that prompted an offer
- **THEN** the offer is not shown again for that message, whichever stage they chose

#### Scenario: A stage set before the message leaves the offer standing

- **WHEN** the newest `stage_set` in the ledger predates the message that disagrees with it
- **THEN** the offer is shown, because the candidate has not yet seen this message

#### Scenario: A newer disagreeing message asks again

- **WHEN** a further classified message arrives after the candidate's last stage change and
  disagrees with the current stage
- **THEN** a fresh offer is made for that message
