## ADDED Requirements

### Requirement: A multi-control question is one field

When several controls answer one question — a checkbox group under a shared
`fieldset`/`legend` or an equivalently labelled container — `read_form` SHALL
report them as a single field carrying the question's label and the options its
controls offer, not as one field per control.

#### Scenario: A checkbox group is one field with options

- **WHEN** a form asks "which country do you reside in" as 30 checkboxes in one fieldset
- **THEN** `read_form` reports one field labelled with the question, offering those 30 options

#### Scenario: The question's label is used, not the options' labels

- **WHEN** the grouped controls are reported
- **THEN** the field's label is the group's question, so the agent addresses the question rather than an individual option

#### Scenario: Unrelated controls are not grouped

- **WHEN** controls share no labelled container that marks them as one question
- **THEN** each is reported as its own field, as before

### Requirement: A grouped field is filled by choosing options

`fill_simple` SHALL fill a grouped field by setting the controls matching the
chosen option texts, and SHALL leave the group untouched when a requested option
is not among those offered.

#### Scenario: Chooses within the group

- **WHEN** a plan targets a grouped field with an option the group offers
- **THEN** the control for that option is set, and the outcome is reported for the group's label

#### Scenario: An option outside the group is refused

- **WHEN** the requested option is not one the group offers
- **THEN** nothing in the group is set and the outcome reports the value was unavailable

### Requirement: The report names questions, not options

The agent's report SHALL name a grouped question once, rather than listing each of
its controls, so a form with a 30-option question does not swamp what the user is
told.

#### Scenario: One entry per question

- **WHEN** a form contains a 30-checkbox question that was not filled
- **THEN** the report names that one question, not its 30 options
