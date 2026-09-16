## Purpose

Guards an in-progress application-form fill against an early, accidental submission —
one triggered by an ordinary field interaction before every field is filled — so the
system verifies what actually happened instead of continuing to fill a form that may
already be gone, or clicking submit a second time on top of a submission that may
already have gone out.

## ADDED Requirements

### Requirement: An early submit signal stops the fill loop

While filling an application form's fields in order, the system SHALL check, after
each text-entry field, whether the form's own submit control is still present,
enabled, and visible on the page. When it is no longer present, or is present but
disabled or hidden, the system SHALL treat the application as possibly already
submitted: it SHALL NOT fill any remaining field, and it SHALL NOT click submit
itself. A control that has been left mounted but disabled or hidden by a same-page
submission is treated the same as one removed outright — neither is usable to click
again.

#### Scenario: The submit control disappears after a text field is filled

- **WHEN** the submit control is no longer present on the page immediately after a
  text-entry field was filled, with fields still remaining in the plan
- **THEN** the system stops filling the remaining fields
- **AND** the system does not click the submit control itself

#### Scenario: The submit control is left disabled or hidden after a text field is filled

- **WHEN** the submit control is still present on the page immediately after a
  text-entry field was filled, but is disabled or hidden
- **THEN** the system stops filling the remaining fields
- **AND** the system does not click the submit control itself

#### Scenario: The submit control is still present, enabled, and visible after a text field is filled

- **WHEN** the submit control is still present, enabled, and visible on the page
  immediately after a text-entry field was filled
- **THEN** the system continues filling the remaining fields exactly as it would
  without this check

### Requirement: An early submit is classified the same way an ordinary submit is

When the fill loop stops because the submit control disappeared early, the system
SHALL determine the outcome using the same confirmed/refused/unconfirmed
classification it uses after its own deliberate submit click, rather than reporting a
plain error or leaving the outcome undetermined.

#### Scenario: An early submit that carries through to confirmation

- **WHEN** the fill loop stops because the submit control disappeared early
- **AND** the page goes on to show the confirmation text a completed submission shows
- **THEN** the system reports the attempt as confirmed, the same as it would after its
  own submit click succeeding

#### Scenario: An early submit whose outcome cannot be determined

- **WHEN** the fill loop stops because the submit control disappeared early
- **AND** the page shows neither a confirmation nor a refusal within the system's
  normal waiting window
- **THEN** the system reports the attempt as unconfirmed, never as a plain error and
  never as a silently assumed success

### Requirement: The check is scoped to field interactions that can trigger a submit

The system SHALL perform the submit-control presence check only after a field
interaction capable of triggering the form's own submit behavior. Filling a field
whose interaction cannot do so SHALL NOT be followed by this check.

#### Scenario: A non-text field fill is not followed by the check

- **WHEN** the system fills a field whose interaction does not send a key event
  capable of triggering the form's own submit behavior
- **THEN** the system does not perform the submit-control presence check after that
  field

### Requirement: A fill that never triggers an early submit is unaffected

When no field interaction causes the submit control to disappear early, the system's
behavior SHALL be unchanged: every field in the plan is filled, and the system then
clicks the submit control itself exactly once.

#### Scenario: A normal fill reaches the loop's own submit click

- **WHEN** every field in the plan fills without the submit control ever
  disappearing early
- **THEN** the system fills every field
- **AND** the system then clicks the submit control itself exactly once
