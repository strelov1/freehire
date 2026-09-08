## Purpose

Ensures that when an ATS application form asks for a cover letter as a free-text field, auto-apply submits the candidate's own tailored letter for that job rather than a generic short answer, whenever a tailored letter already exists.

## ADDED Requirements

### Requirement: A cover-letter-semantic field is recognized distinctly from other free-text questions

The form resolver SHALL identify a free-text field as cover-letter-semantic when its known
identifier or label matches the cover-letter vocabulary (e.g. a field named `cover_letter_text`
or labeled as requesting a cover letter). A field that does not match SHALL continue to be
resolved as an ordinary free-text question.

#### Scenario: A form's dedicated cover-letter field is recognized

- **WHEN** an application form is resolved and one of its fields is identified as a cover-letter field
- **THEN** that field is handled by the cover-letter-reuse path, not the generic free-text drafter

#### Scenario: An unrelated free-text field is unaffected

- **WHEN** an application form's free-text field asks a question other than for a cover letter (e.g. "Why do you want to work here?")
- **THEN** that field continues to be resolved by the existing generic drafting path

#### Scenario: A non-free-text field matching the cover-letter vocabulary is unaffected

- **WHEN** an application form's `select` or `radio` field's identifier or label happens to
  match the cover-letter vocabulary (e.g. a "Cover Letter" yes/no dropdown)
- **THEN** that field is NOT treated as cover-letter-semantic for reuse purposes — it
  continues to be resolved by the existing generic drafting path, since the letter's own
  prose is never one of the platform's own offered option labels

### Requirement: An existing tailored cover letter is reused for the matching job

When a cover-letter-semantic field is resolved for a (candidate, job) pair that already has a
drafted cover letter with a non-blank body, the system SHALL use that letter's content verbatim
as the field's answer instead of generating a new generic answer. This package's form model
carries no field-length constraint for a free-text question (matching how any other free-text
answer is already used verbatim, per `matchOption`'s own "no options → text taken verbatim"
rule) — there is nothing to bound against, so none is invented here.

#### Scenario: A tailored letter exists for the job being applied to

- **WHEN** auto-apply resolves a cover-letter field for a job the candidate has already had a cover letter drafted for
- **THEN** the field's answer is the candidate's existing cover letter for that job, verbatim, and the generic drafter is not invoked for that field

#### Scenario: A stored letter with a blank body is treated as no letter

- **WHEN** auto-apply resolves a cover-letter field for a job whose stored letter row has a
  blank (empty or whitespace-only) body
- **THEN** the field is resolved by the generic drafter, exactly as when no letter exists yet
  — a required field is never answered with a blank string

### Requirement: The generic drafter remains the fallback when no tailored letter exists

When no cover letter has been drafted yet for the (candidate, job) pair, resolving a
cover-letter-semantic field SHALL fall back to the existing generic grounded-drafting behavior,
unchanged from today.

#### Scenario: No cover letter exists yet for the job

- **WHEN** auto-apply resolves a cover-letter field for a job the candidate has no drafted cover letter for
- **THEN** the field is resolved by the existing generic drafter, exactly as before this change
