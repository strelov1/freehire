## ADDED Requirements

### Requirement: Agent maps the profile to the form

Given the current form (via `read_form`) and the signed-in user's profile
(via a hire API), the agent SHALL produce a `{label → value}` plan for the
standard fields it can confidently fill, and SHALL leave a field unmapped rather
than guess a value it cannot justify from the profile.

#### Scenario: Standard fields mapped from the profile

- **WHEN** the agent autofills a form containing name / email / phone / location fields and the profile has those values
- **THEN** the plan maps each of those fields to the profile value

#### Scenario: Unknown field left unmapped

- **WHEN** a field has no basis in the profile
- **THEN** the agent leaves it out of the plan (no fabricated value)

### Requirement: Agent fills standard fields end-to-end

The agent SHALL fill the mapped standard fields through the wire (`fill_simple`)
and SHALL report what was filled. Custom-widget fields (comboboxes) are out of
scope for this change and are reported as not-yet-supported, not filled with a
wrong value.

#### Scenario: End-to-end fill of standard fields

- **WHEN** the user triggers autofill on a form
- **THEN** the agent reads the form, maps the profile, fills the standard fields via the wire, and reports the result

#### Scenario: Comboboxes reported, not corrupted

- **WHEN** the form contains custom-widget comboboxes
- **THEN** the agent reports them as not-yet-fillable and does not write wrong values into them
