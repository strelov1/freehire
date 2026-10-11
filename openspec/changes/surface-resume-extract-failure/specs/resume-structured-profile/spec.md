## ADDED Requirements

### Requirement: A failed structured-résumé extraction can be retried without re-uploading

The system SHALL offer an authenticated endpoint that re-runs structured-résumé extraction from
the candidate's ALREADY-STORED résumé upload, without requiring them to submit the file again.
The endpoint SHALL re-derive plain text from the stored upload the same way every other reader of
it does, and run the same background derivation the upload path uses. It SHALL fail with a
client error when the candidate has no stored résumé to retry.

This endpoint changes nothing about WHEN extraction succeeds or fails — the same fail-closed
rules (PII detector required, LLM required) from "Structured résumé is extracted best-effort on
upload" apply unchanged; this only gives the candidate a way to ask for another attempt.

#### Scenario: Retry re-derives text from the stored upload

- **WHEN** the candidate calls the retry endpoint and has a résumé stored
- **THEN** the system fetches the stored bytes, re-derives text from them, and runs the same
  background structured-extraction the upload path runs

#### Scenario: Retry with no stored résumé is rejected

- **WHEN** the candidate calls the retry endpoint with no résumé ever uploaded
- **THEN** the request is rejected with a client error, and nothing is derived

#### Scenario: Retry still fails closed without the PII detector

- **WHEN** the candidate calls the retry endpoint while the PII detector is unconfigured or
  unavailable
- **THEN** no CV text is sent to the LLM and no structured résumé is persisted, exactly as an
  ordinary upload's extraction would fail closed
