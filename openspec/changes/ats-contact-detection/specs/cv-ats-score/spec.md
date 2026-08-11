## MODIFIED Requirements

### Requirement: Deterministic CV ATS-readiness score

The system SHALL compute an ATS-readiness score for a profile's CV from the CV's
plain text, using deterministic structural checks: machine-readability (near-empty
extracted text ⇒ the CV is a scan/image ⇒ hard fail), presence of contact info
(email and phone, **each scored as its own line item**), standard sections
(Experience/Education/Skills headings, EN and RU), dates, a sane length band, and
bullet usage. Each check SHALL carry a status (pass/warn/fail), a human label, and a
concrete fix. The score SHALL be reproducible (same CV text ⇒ same score) and
require no LLM.

#### Scenario: A scanned/image CV fails machine-readability
- **WHEN** the extracted CV text is near-empty (a scanned or image-only PDF)
- **THEN** the `machine_readable` check is `fail` and the overall readability is low

#### Scenario: A clean text CV scores its structural checks
- **WHEN** a CV has an email, phone, Experience/Skills sections, dates, bullets, and a normal length
- **THEN** those checks are `pass` and readability is high

#### Scenario: Deterministic
- **WHEN** the same CV text is scored twice
- **THEN** the score and checklist are identical

## ADDED Requirements

### Requirement: Email and phone scored as separate line items

The deterministic score SHALL carry email presence and phone presence as two
independent line items, each with its own status, label and fix. Neither item's
status SHALL depend on the other's. Their combined point value SHALL equal what the
single combined contact item was worth, so that widening detection cannot inflate a
CV's score beyond its previous ceiling.

A single item covering both details cannot report which one is absent, and so
instructs a candidate to add a detail their CV already contains.

#### Scenario: A CV with an email but no phone is told only about the phone
- **WHEN** a CV contains an email address and no recognizable phone number
- **THEN** the email item is `pass` and the phone item is `warn`
- **AND** the phone item's fix asks for a phone number and does not ask for an email

#### Scenario: A CV with a phone but no email is told only about the email
- **WHEN** a CV contains a phone number and no email address
- **THEN** the phone item is `pass` and the email item is `warn`

#### Scenario: Contact detail points are conserved
- **WHEN** a CV contains both an email and a phone number
- **THEN** the two items together contribute the same points as the previous combined contact item

### Requirement: Phone detection spans international, national and unseparated formats

Phone detection SHALL recognize a number written in the conventions candidates
actually use, not only `+`-prefixed international form and US grouping. It SHALL
accept, at minimum: an unseparated national digit run (for example an 11-digit
Brazilian mobile), a national number with a non-three-digit area code in
parentheses, separator styles using spaces, dots or hyphens, and a country prefix
written as `+` or as `00`.

Detection SHALL reject digit runs a CV is otherwise full of: four-digit years, year
ranges, calendar dates, percentages and currency figures, and digit runs longer than
an internationally valid number.

#### Scenario: An unseparated national mobile is detected
- **WHEN** a CV writes its phone as a bare run of 11 digits with no separators or prefix
- **THEN** the phone item is `pass`

#### Scenario: A two-digit parenthesized area code is detected
- **WHEN** a CV writes its phone as `(NN) NNNNN-NNNN`
- **THEN** the phone item is `pass`

#### Scenario: Previously recognized formats keep working
- **WHEN** a CV writes its phone in `+`-prefixed international form, as `(NNN) NNN-NNNN`, or as `NNN-NNN-NNNN`
- **THEN** the phone item is `pass`

#### Scenario: A year range is not a phone number
- **WHEN** a CV contains employment date ranges such as `2019 - 2024` and no phone number
- **THEN** the phone item is `warn`

#### Scenario: Metrics and identifiers are not phone numbers
- **WHEN** a CV contains figures such as a revenue amount or a long numeric identifier and no phone number
- **THEN** the phone item is `warn`
