## ADDED Requirements

### Requirement: The store holds six candidate-stated screening answers, each optional

The system SHALL persist, one row per user, the six answers a candidate can state directly
and no CV can supply: the countries they are authorized to work in, whether they need visa
sponsorship, their desired salary (amount, currency, period), their notice period in days,
whether they are willing to relocate, and whether they are 18 or older. Each field SHALL be
independently nullable — a candidate who has not stated one of the six still has the others
served and usable.

Country codes SHALL be validated against the existing country dictionary
(`internal/location`), an unrecognized code rejected rather than stored, following the
repository's dict-only rule. Salary period SHALL be validated against the existing closed
enum `vocab.SalaryPeriodValues`. Salary currency has no closed dictionary in this codebase
(`internal/vocab` documents it as an open ISO-standard field), so it SHALL instead be
validated as a well-formed ISO 4217 code (three uppercase letters) — a narrower guarantee
than dictionary recognition, but the only one available.

#### Scenario: A candidate states only one answer

- **WHEN** a candidate sets their notice period and states nothing else
- **THEN** the stored record has the notice period set and every other field remains
  unstated

#### Scenario: An unrecognized country code is rejected

- **WHEN** a write names a country code the dictionary does not recognize
- **THEN** the write is rejected and no field of the record changes

#### Scenario: A malformed salary currency is rejected

- **WHEN** a write names a currency that is not a three-letter ISO 4217 code
- **THEN** the write is rejected and no field of the record changes

#### Scenario: A candidate who has stated nothing reads as fully unstated

- **WHEN** a candidate who has never set any screening answer is read
- **THEN** every field is absent rather than defaulted to a guessed value

### Requirement: The candidate can read and update their own screening answers

The system SHALL expose an authenticated endpoint for the caller to read their current
screening answers and another to update any subset of them, so the manual-edit surface on
the profile page can both display and change the record. A write updates only the fields
it names; fields it omits are left as they were.

#### Scenario: A partial update leaves other fields untouched

- **WHEN** a caller who already has a stored desired salary updates only their
  willingness to relocate
- **THEN** the desired salary is unchanged and the willingness to relocate reflects the
  update

#### Scenario: Reading returns the caller's own record only

- **WHEN** an authenticated caller reads their screening answers
- **THEN** the response reflects only their own stored record, never another user's

### Requirement: The assistant can set screening answers from what the candidate states in chat

The system SHALL offer an assistant tool that writes any subset of the six fields from a
turn in which the candidate states them, calling the same service the manual-edit endpoint
calls. The tool SHALL accept a partial set of fields, the same as the manual-edit endpoint,
and SHALL return the fields it wrote so the assistant can confirm them back to the
candidate. The tool carries no inference step: it writes only values present in its
arguments, and nothing about ambient conversation the candidate did not state.

#### Scenario: The candidate states one fact in passing

- **WHEN** a candidate mentions in chat that they would need visa sponsorship, and states
  nothing else
- **THEN** the assistant tool writes only the visa sponsorship field and reports that value
  back

#### Scenario: A malformed value is rejected with an actionable error

- **WHEN** the assistant tool is called with a country code or currency the dictionaries do
  not recognize
- **THEN** the call fails with a message naming the invalid value, so the model can correct
  itself
