## MODIFIED Requirements

### Requirement: Tracking board marks a silent application

The web SPA's tracking board SHALL mark each application card with its silence
state, showing the days silent for an application that is `silent` and an
invitation to confirm the pending mail for one that is `unconfirmed`. A card in
the `active` state and a card for a terminal application SHALL carry no silence
marker, so the marker means something when it appears.

The marker SHALL be an indicator, not a control: a card carries no controls, and
the follow-up draft a silent application is owed SHALL be offered in the opened
application instead. Once a follow-up has been recorded the card SHALL say that
the application was chased and when, without dropping or softening the silence
marker. "Chased, still nothing" and "nobody has done anything about it" are
different situations, and the card MUST NOT render them the same way.

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

#### Scenario: The draft is offered in the opened application

- **WHEN** a card's application is `silent` and the user opens it
- **THEN** the opened application offers a follow-up draft, and the card itself
  offers no control

#### Scenario: A chased card keeps its silence marker

- **WHEN** a card's application is `silent` and a follow-up has been recorded
- **THEN** the card still shows the silence marker, and additionally reports that it was chased
