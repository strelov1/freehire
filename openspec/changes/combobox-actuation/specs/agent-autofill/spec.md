## MODIFIED Requirements

### Requirement: Agent maps the profile to the form

Given the current form (via `read_form`) and the signed-in user's profile
(via a hire API), the agent SHALL produce a plan for the fields it can
confidently fill, and SHALL leave a field unmapped rather than guess a value it
cannot justify from the profile.

For a custom-widget field the plan names an **option the widget offers**, not a
free-text value, and that option SHALL be justified from the profile on the same
terms as a typed value.

#### Scenario: Standard fields mapped from the profile

- **WHEN** the agent autofills a form containing name / email / phone / location fields and the profile has those values
- **THEN** the plan maps each of those fields to the profile value

#### Scenario: Unknown field left unmapped

- **WHEN** a field has no basis in the profile
- **THEN** the agent leaves it out of the plan (no fabricated value)

#### Scenario: A widget option is chosen from the profile

- **WHEN** a country combobox offers "Germany" and the profile's location is "Berlin, Germany"
- **THEN** the plan selects that option

#### Scenario: A question the profile does not answer is left alone

- **WHEN** a widget asks something the profile says nothing about (visa sponsorship, notice period)
- **THEN** no option is selected and the field is reported for the user to answer

### Requirement: Agent fills standard fields end-to-end

The agent SHALL fill the mapped fields through the wire and SHALL report what was
filled. For a custom-widget field it SHALL drive the widget — open, read the
offered options, select, verify — and SHALL count the field as filled only when
verification confirms the committed value.

#### Scenario: End-to-end fill of standard fields

- **WHEN** the user triggers autofill on a form
- **THEN** the agent reads the form, maps the profile, fills the standard fields via the wire, and reports the result

#### Scenario: A widget is filled by choosing an offered option

- **WHEN** the form contains a custom-widget combobox whose options the profile justifies
- **THEN** the agent selects that option through the wire and reports the field filled

#### Scenario: An unverified widget write is not a fill

- **WHEN** a widget commits something other than the selected option
- **THEN** the agent reports the field as not filled, so the user sees it still needs answering

#### Scenario: An undrivable widget is reported, not corrupted

- **WHEN** a widget cannot be opened or driven
- **THEN** the agent reports it as not-yet-fillable and writes nothing into it
