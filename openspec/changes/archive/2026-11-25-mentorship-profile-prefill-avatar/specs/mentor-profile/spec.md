## MODIFIED Requirements

### Requirement: A mentor profile is public and named

A published mentor profile SHALL show the mentor's display name, headline, company and
topics to any visitor, signed in or not. A mentor profile SHALL NOT be anonymised in the
manner of a referral offer.

The display name SHALL be a field of the PROFILE, not read from the account: `users`
carries no name at all — only an address and a username, and a username is an address
rather than a name.

A profile SHALL carry a `show_photo` opt-in, defaulting to off, that controls whether
the mentor's account headshot is served publicly (see the photo requirement below). A
profile that has never opted in SHALL remain exactly as pictureless as this requirement
originally described.

#### Scenario: An anonymous visitor reads a published profile

- **WHEN** a signed-out visitor opens an approved mentor's public URL
- **THEN** the response carries the mentor's name, headline, company and topics

#### Scenario: A profile without a name is refused

- **WHEN** a profile is submitted with an empty or whitespace-only display name
- **THEN** the submission is refused
- **AND** no profile row is written

#### Scenario: A pending profile is not publicly readable

- **WHEN** a signed-out visitor requests the public URL of a profile whose status is
  `pending`, `rejected` or `paused`
- **THEN** the system responds as it would for a profile that does not exist
- **AND** the owner and a moderator can still read it through their own routes

## ADDED Requirements

### Requirement: A mentor's photo is shown only with their explicit opt-in

The system SHALL serve a mentor's account headshot on their public directory card and
profile page only when that mentor's own `show_photo` field is set to true. Setting
`show_photo` SHALL be an ordinary, mentor-controlled edit to their own profile, off by
default for both a newly created and a pre-existing profile. Opting in SHALL NOT copy
or duplicate the photo into mentor-specific storage — the account's existing CV
headshot remains the single stored copy, and turning the opt-in back off SHALL stop it
being served publicly with no further action needed.

The photo SHALL be reachable only for a profile that is currently public under the
existing publication rule (`approved` and not paused): opting in does not create a way
to read a photo for a profile that itself is not publicly readable.

#### Scenario: An opted-in, approved mentor's photo is publicly served

- **WHEN** an approved, unpaused mentor with `show_photo` set to true has a stored
  account headshot
- **THEN** a signed-out visitor's request for that mentor's public photo succeeds

#### Scenario: A mentor who has not opted in serves no photo

- **WHEN** a mentor has `show_photo` set to false, whether or not they have a stored
  account headshot
- **THEN** a request for their public photo does not serve it

#### Scenario: Opting in without a stored headshot serves no photo

- **WHEN** a mentor sets `show_photo` to true but has no stored account headshot
- **THEN** a request for their public photo does not serve it

#### Scenario: A non-public profile's photo is not reachable regardless of opt-in

- **WHEN** a mentor profile is `pending`, `rejected`, `paused`, or withdrawn, even with
  `show_photo` set to true and a stored headshot
- **THEN** a request for that mentor's public photo does not serve it
