## ADDED Requirements

### Requirement: PII detection combines a regex floor with a local model

The system SHALL detect PII spans in CV text by unioning a deterministic regex layer with
spans returned by a local `openai/privacy-filter` detector. The regex layer SHALL detect
email addresses, phone numbers, URLs, and `@handle`s; the model SHALL contribute
`PERSON`/`ADDRESS`/`LOCATION` spans that the regex layer cannot reliably find. A phone match
that is a bare `YYYY-YYYY` year range MUST NOT be treated as a phone number.

#### Scenario: Regex catches direct contact identifiers

- **WHEN** CV text contains an email, phone, URL, or `@handle`
- **THEN** each is detected as a PII span regardless of the model result

#### Scenario: Model recovers a name the regex cannot

- **WHEN** the CV surname appears only inside the email local-part or a URL slug (no plain-text name line)
- **THEN** the model detector contributes a `PERSON` span that covers it

#### Scenario: Date range is not a phone number

- **WHEN** CV text contains `2012-2016`
- **THEN** it is not detected as a phone span

### Requirement: Redaction is reversible via numbered placeholders

The system SHALL produce a `Redactor` from the detected spans that replaces each distinct PII
value with a stable, numbered placeholder (e.g. `[REDACTED_NAME]`, `[REDACTED_EMAIL_1]`).
`Redact` SHALL replace values on the way into a prompt and `Restore` SHALL map placeholders
back to the originals, such that `Restore(Redact(text))` reproduces the original for every
detected value. Replacement SHALL occur on word boundaries, and a known/full value SHALL take
priority over a shorter overlapping token to bound over-redaction.

#### Scenario: Round-trip restores the original

- **WHEN** a text is masked with `Redact` and then passed through `Restore`
- **THEN** every detected PII value is returned to its original form

#### Scenario: Distinct values get distinct placeholders

- **WHEN** two different emails appear in the CV
- **THEN** they receive distinct numbered placeholders that restore independently

### Requirement: Non-PII context is preserved

The redactor SHALL NOT mask employer names, universities, job titles, skills, or city/country
context, so downstream fit reasoning retains the signals it needs.

#### Scenario: Employers and cities stay visible

- **WHEN** a CV names employers and cities alongside the candidate's contact block
- **THEN** only the contact identifiers and the person/address spans are masked; employers and cities are left intact

### Requirement: Detection is fail-closed

When the detector is unconfigured or unavailable, building a `Redactor` SHALL return an error
rather than a partial (regex-only) redactor, so callers can refuse to send the CV to the LLM.
The regex layer alone is NOT a sufficient fallback for the name.

#### Scenario: Unavailable detector yields an error

- **WHEN** the `PII_FILTER_URL` detector is unset or the call fails
- **THEN** `Build` returns an error and no redactor is produced
