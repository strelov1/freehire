## Purpose

Lets the auto-apply system recognize, before it attempts to fill or submit anything, that a
job posting's application form cannot be safely read or driven at all — because its layout is
not one this system knows how to read, or because it is protected by a human-verification
challenge — and record that as a clean, non-retryable outcome rather than a transient error.

## ADDED Requirements

### Requirement: An unrecognized form layout parks the attempt instead of erroring

The system SHALL detect, without guessing, when a job posting's rendered application form
does not match any layout it knows how to read, and SHALL record the attempt as unresolved
with a reason identifying that the form's layout was not recognized. The system SHALL NOT
report this as a generic failure indistinguishable from a transient error (a network hiccup,
a temporarily unreachable page), and SHALL NOT attempt to fill or submit anything on such a
form.

#### Scenario: A job board's custom application-page layout is not recognized

- **WHEN** a queued attempt's job posting renders an application form whose structure does
  not match any layout the system knows how to read
- **THEN** the attempt is recorded as unresolved with a reason identifying that the form's
  layout could not be recognized
- **AND** no application is submitted, and nothing on the employer's page is filled in

### Requirement: A challenge-protected form parks the attempt instead of erroring

The system SHALL detect, before attempting to fill or submit anything, when a job posting's
application form is protected by a human-verification challenge (such as a CAPTCHA) that this
system cannot pass, and SHALL record the attempt as unresolved with a reason identifying the
challenge. This applies regardless of whether the employer's platform is one this system
otherwise knows how to fill and submit for.

#### Scenario: A form the system could otherwise fill is protected by a challenge

- **WHEN** a queued attempt's job posting renders an application form that is otherwise
  readable, but the form itself is protected by a human-verification challenge
- **THEN** the attempt is recorded as unresolved with a reason identifying the challenge
- **AND** no application is submitted, and nothing on the employer's page is filled in

### Requirement: An unscannable-form outcome does not consume the transient-failure retry budget

The system SHALL distinguish an attempt parked because its form could not be recognized or is
challenge-protected from an attempt that failed for a transient reason. The system SHALL NOT
apply its transient-failure retry or dead-letter accounting to an unscannable-form outcome —
retrying against a form that will not change shape or stop being challenge-protected cannot
succeed, so it SHALL be treated the same as any other attempt correctly declining to guess.

#### Scenario: A run does not dead-letter an attempt parked for an unrecognized form

- **WHEN** an attempt has been recorded as unresolved because its form's layout was not
  recognized or it is challenge-protected
- **THEN** a subsequent run does not retry that attempt as a transient failure
- **AND** the attempt is not counted toward the threshold that dead-letters a repeatedly
  failing attempt
