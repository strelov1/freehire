# CV extraction and contact handling

## Purpose

Extract structured CV facts directly from bounded original text. The former local PII
masking service is retired; see [the decision](../../../.out-of-scope/pii-filter.md).

## Requirements

### Requirement: One extraction path reads the original CV

The system SHALL send bounded original CV text to the configured LLM gateway with a schema
covering contacts, professional experience, education and skills. No separate PII detector
SHALL be required. The model SHALL be instructed to copy source facts, return null for
missing facts, preserve name spelling and employment dates, and omit contact details from
professional prose. The result SHALL be sanitized and dates checked before persistence.

#### Scenario: Contacts and experience are extracted together

- **WHEN** a CV states a name, contacts and dated employment
- **THEN** extraction reads them together without masking or a detector request
- **AND** absent contacts remain empty

### Requirement: Downstream contact boundaries remain

The contact-free professional projection SHALL continue to omit the candidate's full name,
email, phone and contact links. Public/anonymous projections SHALL retain their existing
additional restrictions. Upload-stamp guards, caller attribution and error handling SHALL
remain in effect.

#### Scenario: An agent reads the derived profile

- **WHEN** a profile is served through its professional projection
- **THEN** contact fields are not included

### Requirement: A missing LLM disables structured extraction

#### Scenario: Extraction is unconfigured

- **WHEN** the LLM client is absent
- **THEN** structured extraction is disabled and deterministic parsing remains available
