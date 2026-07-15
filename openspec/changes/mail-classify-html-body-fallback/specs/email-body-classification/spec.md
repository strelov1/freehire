## ADDED Requirements

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
