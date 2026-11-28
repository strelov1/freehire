## ADDED Requirements

### Requirement: A job entry's Company field offers catalogue suggestions while accepting free text
When adding or editing a job-kind employment, the Company field SHALL offer matching suggestions from the company catalogue as the candidate types, and SHALL still accept a company name that matches no catalogue entry.

#### Scenario: A matching company is suggested
- **WHEN** the candidate types into the Company field of the add-job or edit-job form and at least one company catalogue entry matches
- **THEN** the system shows matching companies as selectable suggestions

#### Scenario: Picking a suggestion fills the field
- **WHEN** the candidate selects a suggested company
- **THEN** the Company field is set to that company's canonical name

#### Scenario: A company outside the catalogue is still accepted
- **WHEN** the candidate types a company name that matches no catalogue entry and saves the form without picking a suggestion
- **THEN** the employment is saved with the typed text as the company name

### Requirement: A job-kind employment can be marked as current
The add-job and edit-job forms SHALL let the candidate mark a job-kind employment as current ("I currently work here"), which SHALL be mutually exclusive with an end date.

#### Scenario: Marking a job as current clears the end date
- **WHEN** the candidate checks "I currently work here" on a job-kind employment
- **THEN** the end date input is hidden and the employment is saved as current with no end date

#### Scenario: Unmarking a job as current restores the end date
- **WHEN** the candidate unchecks "I currently work here" on an employment previously marked current
- **THEN** an end date input is shown again and the employment is saved as not current

#### Scenario: A project-kind entry has no current-employment control
- **WHEN** the candidate adds or edits a project-kind entry
- **THEN** no "I currently work here" control is shown, since only job-kind employments support it
