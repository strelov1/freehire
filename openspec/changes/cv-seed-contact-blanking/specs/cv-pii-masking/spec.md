## ADDED Requirements

### Requirement: Contacts recovered from detection include candidate location

The redactor's recovered `Contacts` SHALL include the candidate's location taken from the first plausible `ADDRESS` span, alongside the existing first name, email, phone and collected links. Callers that fill structured-résumé contact fields from `Contacts` (résumé extraction) MUST therefore be able to restore residence without asking the LLM to invent it from a redacted prompt.

An `ADDRESS` span remains masked on the way into the extraction prompt; recovery into `Contacts` is how the original value returns to the structured résumé after extraction. Employer and role city lines that are not tagged as address spans continue to stay visible in the redacted text for semantic extraction, as required by the non-PII-context rule.

#### Scenario: Address span becomes Contacts.location

- **WHEN** the detector returns an `ADDRESS` span for the candidate's residence line
- **THEN** `Contacts` carries that value as location (first plausible address wins)

#### Scenario: No address span leaves location empty

- **WHEN** the CV states no residence the detector tags as `ADDRESS`
- **THEN** `Contacts.location` is empty and no fabricated location is invented
