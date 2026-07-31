## ADDED Requirements

### Requirement: An opened application offers a rehearsal

The opened application SHALL offer a rehearsal whatever its stage, and starting one
SHALL create an assistant session bound to that application's vacancy. The session
SHALL carry the `interview` preset and no CV binding, because a rehearsal reads a CV
but never edits one. The creating endpoint SHALL resolve the application through the
caller's own tracking record, so an application the caller does not own is reported as
missing rather than as forbidden.

An application whose posting the catalogue no longer holds SHALL NOT offer a
rehearsal: the session is bound to a vacancy, and there is none to bind.

#### Scenario: Rehearsal offered whatever the stage

- **WHEN** the candidate opens an application from the tracking board
- **THEN** it offers to start a rehearsal, whether it sits in `applied`, `screening`,
  `interview` or any other stage

#### Scenario: The session is bound to the vacancy and to no CV

- **WHEN** a rehearsal is started from an application
- **THEN** a session is created with the `interview` preset, carrying that application's vacancy and no CV binding

#### Scenario: Another candidate's application cannot be rehearsed

- **WHEN** a caller starts a rehearsal for a vacancy they have no application against
- **THEN** the request is answered as not found, and no session is created

#### Scenario: No posting, no rehearsal

- **WHEN** the candidate opens an application whose posting the catalogue no longer
  holds
- **THEN** no rehearsal is offered

## REMOVED Requirements

### Requirement: An application at the interview stage offers a rehearsal

**Reason**: Both halves of this requirement moved. The offer sat on the board card,
and a card now carries no controls — it is dragged and opened, and an interactive
element on its surface is what stopped it being dragged at all. The stage gate went
with it: it existed because a card has room for one or two controls and one that
appears everywhere stops meaning anything, which is not true of the opened
application. The server never gated a rehearsal, so nothing is loosened here that was
ever enforced — a candidate who wants to prepare a week early now can.

**Migration**: Replaced by "An opened application offers a rehearsal". The endpoint,
the preset, the CV binding and the ownership rule are unchanged; only where the offer
appears and which applications carry it have moved.
