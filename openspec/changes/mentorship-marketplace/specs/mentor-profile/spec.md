## ADDED Requirements

### Requirement: A mentor profile is owned by one account and names one company

The system SHALL hold at most one mentor profile per user account, and that profile
SHALL name exactly one company by `companies.slug` — the same key `jobs.company_slug`
carries. A profile SHALL carry a stable public slug used in its URL, an IANA timezone,
a headline, a biography, a topic list, a language list, and the session parameters
(duration, buffers, minimum notice, booking horizon, meeting link).

#### Scenario: A second profile for the same account is refused

- **WHEN** an account that already has a mentor profile submits another
- **THEN** the system refuses with a conflict rather than creating a second profile

#### Scenario: A profile naming an unknown company is refused

- **WHEN** a profile is submitted with a `company_slug` no `companies` row carries
- **THEN** the system refuses with a not-found error naming the company
- **AND** no profile row is written

### Requirement: A mentor profile is public and named

A published mentor profile SHALL show the mentor's display name, headline, company and
topics to any visitor, signed in or not. A mentor profile SHALL NOT be anonymised in the
manner of a referral offer.

The display name SHALL be a field of the PROFILE, not read from the account: `users`
carries no name at all — only an address and a username, and a username is an address
rather than a name.

An avatar is NOT part of this requirement. Serving one means a public image endpoint
with its own caching, sizing and abuse surface, and the account's stored headshot is a
CV photo a mentor may not want here. It belongs with the screens that display it; until
then a profile is named but not pictured, which is enough to stop it being anonymous.

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

### Requirement: A profile reaches the public only through manual moderation

A submitted profile SHALL begin at status `pending` and SHALL become publicly visible
only when a moderator sets it to `approved`. The system SHALL NOT infer approval from
any other signal, including an approved `referral_offers` row for the same account and
company. A moderator SHALL be able to reject a profile and SHALL be able to move an
approved profile back out of the public listing.

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

### Requirement: A mentor can pause without losing their profile

A mentor SHALL be able to set an approved profile to `paused` and back to `approved`
without moderation. A paused profile SHALL disappear from the directory and SHALL
offer no slots, while its already-confirmed bookings SHALL stand.

#### Scenario: Pausing hides the profile but keeps booked sessions

- **WHEN** an approved mentor with one confirmed future booking pauses their profile
- **THEN** the profile leaves the public directory and its slot endpoint offers nothing
- **AND** the confirmed booking remains confirmed and both parties keep their invitation

#### Scenario: Resuming needs no moderator

- **WHEN** a paused mentor sets their profile back to active
- **THEN** the status returns to `approved` without entering the moderation queue

### Requirement: The public directory lists approved mentors and can be narrowed

The system SHALL serve a public directory of approved, unpaused mentor profiles, and
SHALL allow it to be narrowed by company, by topic and by language. Following the
project's dropped-filter rule, the directory SHALL report any query parameter it did
not read in `meta.ignored_params`, and SHALL omit that key when there are none.

#### Scenario: The directory excludes profiles that are not publishable

- **WHEN** a visitor lists the mentor directory
- **THEN** only profiles whose status is `approved` and which are not paused appear

#### Scenario: An unrecognised filter is reported, not silently ignored

- **WHEN** a visitor requests the directory with a parameter the endpoint does not read
- **THEN** the results are unnarrowed by it
- **AND** `meta.ignored_params` names that parameter

### Requirement: A vacancy and a company page lead to their mentors

Where the catalogue holds an approved mentor for a company, the system SHALL expose
that fact on that company's vacancies and on the company page, so a seeker reading a
posting can reach a mentor at that employer.

The answer SHALL come from the directory narrowed to that company rather than from a
separate "has a mentor?" endpoint. A second way to ask means a second copy of the
publication predicate, and two copies of a predicate drift.

#### Scenario: A vacancy at a company with a mentor offers the entry point

- **WHEN** a visitor opens a vacancy whose `company_slug` has at least one approved,
  unpaused mentor
- **THEN** the page offers a route to that company's mentors

#### Scenario: A vacancy at a company without mentors offers nothing

- **WHEN** a visitor opens a vacancy whose company has no approved, unpaused mentor
- **THEN** no mentorship entry point is rendered

### Requirement: Withdrawing a profile preserves booking history

A mentor SHALL be able to withdraw their profile. Withdrawal SHALL cancel every
confirmed future booking and notify each affected seeker, and SHALL retain past
bookings as history rather than deleting them.

The profile row SHALL be MARKED withdrawn rather than deleted. Bookings and reviews
reference it with `ON DELETE CASCADE`, so a delete would erase every session that ever
happened — the opposite of what this requirement asks for. A withdrawn profile SHALL
leave the directory and the public read, exactly as a paused one does.

#### Scenario: Withdrawal cancels the future and keeps the past

- **WHEN** a mentor with one past completed booking and two confirmed future bookings
  withdraws their profile
- **THEN** both future bookings become cancelled and both seekers are notified
- **AND** the completed booking is still readable in each party's history
- **AND** its review is still readable

#### Scenario: A withdrawn profile is no longer public

- **WHEN** a mentor withdraws
- **THEN** their profile is absent from the directory and from the public read
- **AND** a second withdrawal changes nothing
