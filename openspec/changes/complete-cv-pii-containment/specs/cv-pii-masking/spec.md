## ADDED Requirements

### Requirement: The detector runs only at résumé extraction

The local PII detector SHALL be invoked at exactly one point — résumé extraction on upload —
where it de-identifies the raw CV before the extraction LLM sees it. Downstream surfaces (fit
analysis, the ATS qualitative review, CV tailoring) SHALL NOT call the detector or send the raw
CV to a model; they consume de-identified derived data (the structured résumé, or the CV body
without its contact block). This keeps the raw CV → external-model hop to a single, masked place.

#### Scenario: Extraction is the only raw-CV → model path

- **WHEN** a CV is uploaded and later used for fit analysis, an ATS review, and tailoring
- **THEN** the raw CV text is sent to a model only during extraction (masked by the detector), and every downstream surface uses de-identified derived data

#### Scenario: Downstream needs no detector

- **WHEN** the fit analysis or the ATS review runs
- **THEN** it does not invoke the PII detector and does not depend on its availability
