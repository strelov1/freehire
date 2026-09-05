## Purpose

Resolves and fills a job application's résumé/CV file-upload field from a candidate's
approved, tailored CV during a live auto-apply submission attempt — the gap that made every
such field park unconditionally.

## ADDED Requirements

### Requirement: A résumé file field resolves from the entry's approved tailored CV

When a submission attempt's form carries a required or optional résumé/CV file-upload field,
the system SHALL resolve it from the queue entry's approved tailored CV, rendered to a PDF on
demand — never from object storage, and never guessed when the entry carries no approved CV.
An entry reaching this step without an approved CV attached is a defect in the caller (the
runner never claims one), not a condition this resolution handles gracefully.

#### Scenario: A required résumé field is filled from the approved CV

- **WHEN** a submission attempt's form has a required résumé file field and the entry carries
  an approved tailored CV
- **THEN** the field is filled with that CV rendered to a PDF, and the attempt is not parked on
  this field

#### Scenario: A cover-letter file field is not resolved by this path

- **WHEN** a submission attempt's form has a file field that is not the résumé/CV upload
- **THEN** it is left unmapped exactly as before this change — this only closes the résumé gap

### Requirement: A file field still parks when nothing can be attached

The system SHALL park an attempt on a required résumé field when the queue entry's approved CV
cannot be rendered (a renderer failure) — the same "never guess, park instead" rule every other
unresolved field already follows. This is a transient-looking but treated-as-park outcome
deliberately: a render failure needs investigation, not a blind retry against the employer's
live form.

#### Scenario: A render failure parks rather than retries the submission

- **WHEN** the approved tailored CV cannot be rendered to a PDF
- **THEN** the attempt is parked, naming the résumé field as unresolved, rather than retried
  through the ordinary transient-failure path
