## MODIFIED Requirements

### Requirement: The autofill profile is one canonical contact and screening block

The system SHALL serve the browser extension a fixed set of fields assembled server-side, so
the extension holds no rule about where a value comes from: the identity fields `full_name`,
`first_name`, `last_name`, `email`, `phone`, `location`, `linkedin`, `github` and
`portfolio`, plus the six screening answers — authorized countries, visa sponsorship needed,
desired salary, notice period, willingness to relocate, and 18-or-older — formatted as
human-readable strings. `first_name` and `last_name` SHALL be derived by splitting the full
name on whitespace — the first token is the given name, the remainder the family name. The
links SHALL be sorted by host: the first LinkedIn URL, the first GitHub URL, and the first
remaining URL as the portfolio. A field no source states SHALL be absent rather than
guessed, whether it is an identity field or a screening answer.

This block carries the candidate's identity and their own stated screening answers. It still
carries no summary, no work history and no skills — those remain out of scope for the
contact-and-screening section of an application form.

#### Scenario: The block is served with the fields the sources state

- **WHEN** an authenticated caller reads the autofill profile and a source states a name,
  a phone and a LinkedIn URL
- **THEN** the response carries those values, the given and family names split from the
  full name, and empty strings for the fields no source stated

#### Scenario: Links are sorted by host, not by order

- **WHEN** the chosen source states a personal site, then a GitHub URL, then a LinkedIn URL
- **THEN** `linkedin` is the LinkedIn URL, `github` is the GitHub URL, and `portfolio` is
  the personal site

#### Scenario: Screening answers the candidate has stated are included

- **WHEN** a caller has stored a notice period and a willingness to relocate in their
  screening answers
- **THEN** the autofill profile carries both, formatted as human-readable strings, alongside
  whatever identity fields their CV or résumé states

#### Scenario: Unstated screening answers stay absent

- **WHEN** a caller has never set any screening answer
- **THEN** the autofill profile carries their identity fields as usual and no screening
  field, rather than a guessed or default value
