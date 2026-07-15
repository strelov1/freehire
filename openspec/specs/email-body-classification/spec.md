# email-body-classification Specification

## Purpose
TBD - created by archiving change mail-classify-html-body-fallback. Update Purpose after archive.
## Requirements
### Requirement: Classifier reads the readable email body

The mail classifier SHALL be given the email's readable body, selected from its
plain-text and HTML parts, so that HTML-only messages are classified from their
actual content rather than from the subject alone.

The readable body SHALL be the plain-text part when that part contains
non-whitespace text; otherwise it SHALL be the HTML part converted to readable
text; when HTML-to-text conversion fails it SHALL be the raw HTML; when neither
part has content it SHALL be empty.

Selecting the readable body SHALL NOT change how the email is stored: the
plain-text and HTML parts are preserved unchanged for display.

#### Scenario: HTML-only rejection is classified from its body

- **WHEN** an email has an empty plain-text part and an HTML body that says the
  application will not proceed
- **THEN** the classifier receives the HTML stripped to readable text
- **AND** the email is classified as a rejection, not as screening

#### Scenario: Plain-text part is preferred when present

- **WHEN** an email has a non-empty plain-text part and also an HTML part
- **THEN** the classifier receives the plain-text part unchanged

#### Scenario: Whitespace-only plain-text falls back to HTML

- **WHEN** an email's plain-text part contains only whitespace and its HTML part
  has content
- **THEN** the classifier receives the HTML stripped to readable text

#### Scenario: Both parts empty yields an empty body

- **WHEN** an email has neither a plain-text nor an HTML part
- **THEN** the readable body is empty and classification proceeds from the
  sender and subject

### Requirement: Deterministic keyword status fast-path

The classifier SHALL, for an email needing no application disambiguation (no
candidate applications offered), first attempt a deterministic keyword
classification of the status and use it WITHOUT calling the LLM when a confident
status is found. When no confident keyword status is found, or when candidate
disambiguation is required, classification SHALL proceed via the LLM as before.

The keyword classification SHALL be precision-first: it SHALL return a status
only for strong, unambiguous signals and SHALL defer otherwise. A rejection
signalled by an explicit negative phrase SHALL take precedence over an
acknowledgement opener in the same email.

#### Scenario: Confident keyword status skips the LLM

- **WHEN** an email with no candidate applications contains an explicit rejection
  phrase ("we regret to inform you… not to proceed")
- **THEN** it is classified as a rejection without an LLM call

#### Scenario: Ambiguous email defers to the LLM

- **WHEN** an email's status is not clear from strong keywords
- **THEN** the classifier falls back to the LLM

#### Scenario: Candidates present always use the LLM

- **WHEN** candidate applications are offered for disambiguation
- **THEN** the classifier uses the LLM regardless of keyword matches

