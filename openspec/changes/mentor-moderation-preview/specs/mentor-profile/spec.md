## MODIFIED Requirements

### Requirement: A profile reaches the public only through manual moderation

A submitted profile SHALL begin at status `pending` and SHALL become publicly visible
only when a moderator sets it to `approved`. The system SHALL NOT infer approval from
any other signal, including an approved `referral_offers` row for the same account and
company. A moderator SHALL be able to reject a profile and SHALL be able to move an
approved profile back out of the public listing.

The moderation queue's read of a pending profile SHALL include when it was submitted,
and SHALL let a moderator preview the profile's opted-in photo — without exposing it
through the public, slug-keyed photo route, which SHALL continue to answer as though an
unapproved profile does not exist.

#### Scenario: A submitted profile is not yet listed

- **WHEN** a user submits a mentor profile
- **THEN** its status is `pending`
- **AND** it appears in the moderation queue, oldest first
- **AND** it does not appear in the public directory

#### Scenario: An approved referral offer does not auto-approve a profile

- **WHEN** a user with an approved `referral_offers` row for company X submits a
  mentor profile for company X
- **THEN** the profile status is still `pending`
- **AND** the moderation queue marks the corroborating referral offer as evidence

#### Scenario: A moderator approves a profile

- **WHEN** a moderator approves a pending profile
- **THEN** the status becomes `approved` and the deciding moderator and time are recorded
- **AND** the profile appears in the public directory and at its public URL

#### Scenario: The queue reports when a profile was submitted

- **WHEN** a moderator reads the pending queue
- **THEN** each entry carries the time its profile was submitted

#### Scenario: A moderator can preview a pending profile's opted-in photo

- **WHEN** a moderator requests the photo of a pending profile that opted in
  (`show_photo`)
- **THEN** the photo is served to the moderator

#### Scenario: The public photo route still refuses an unapproved profile

- **WHEN** a signed-out visitor requests a pending or rejected profile's photo through
  the public, slug-keyed route
- **THEN** the response is as though the profile does not exist
