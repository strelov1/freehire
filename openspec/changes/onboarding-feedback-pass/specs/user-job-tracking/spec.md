## ADDED Requirements

### Requirement: A saved job appears on the tracking board as Preparing

A tracked row that carries a saved mark and no application stage SHALL be presented on the
tracking board in the `preparing` column.

This is a presentation rule, not a write: saving a job SHALL continue to record only the
saved mark, and no stage SHALL be written on the visitor's behalf. The board previously
discarded these rows when laying out its columns; placing them costs no migration, no
backfill and no second write, and applies to every job saved before the change as well as
after.

An explicit stage SHALL continue to decide the column — a saved row that has been moved to
`interview` stays in `interview`.

#### Scenario: An existing bookmark appears on the board

- **WHEN** a user who saved jobs before this change opens the tracking board
- **THEN** those jobs appear as cards in the Preparing column, without any migration having
      run

#### Scenario: An explicit stage still wins

- **WHEN** a saved job also carries an application stage
- **THEN** the board places it by that stage, not in Preparing

#### Scenario: Saving writes no stage

- **WHEN** a user saves a job
- **THEN** only the saved mark is recorded, and no application stage is written

### Requirement: Saving is a primary, labelled action on a job card

The save control on a job card SHALL be a labelled button rather than an icon-only overlay,
and SHALL read as the card's primary action. The label SHALL name the state the job is in
rather than the action performed, since a long feed has to answer "is this one already
mine" at a glance.

The narrow-column card — the one the assistant chat renders, at roughly 360px — is
exempt and keeps the icon-only overlay. That card drops the blurb and shortens the title
for the same reason; a control that earns a word on a full card has to give it back
there. The exemption is by card width, not by surface, so no caller opts into it.

The control SHALL remain outside the card's link element so that activating it never
navigates to the posting.

#### Scenario: The control is legible without hovering

- **WHEN** a visitor sees a job card in the feed
- **THEN** the save action is a labelled button, not an unlabelled glyph

#### Scenario: Saving does not navigate

- **WHEN** a visitor activates the save control on a card
- **THEN** the job is saved and the browser does not navigate to the posting
