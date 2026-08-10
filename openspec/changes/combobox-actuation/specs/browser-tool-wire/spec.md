## MODIFIED Requirements

### Requirement: read_form primitive

The extension SHALL expose `read_form`, returning the page's fillable controls
with a stable, human-meaningful **label** for each (from `<label>`,
`aria-labelledby`, `aria-label`, placeholder, or name), across frames.

A control the user cannot see SHALL NOT be reported. For a custom-widget
combobox, the report SHALL say so and SHALL carry the options the widget offers
where they can be read without opening it; where they cannot, the agent obtains
them through `combobox.open` + `combobox.options`. Several controls answering one
question SHALL be reported as a single field (see the `field-grouping`
capability).

#### Scenario: Reads labelled fields

- **WHEN** `read_form` runs on a page with a form
- **THEN** it returns each fillable control's label, type, current value, and whether it is a custom-widget combobox

#### Scenario: Reaches iframe'd forms

- **WHEN** the form is inside an iframe
- **THEN** `read_form` includes that frame's fields (tagged by frame)

#### Scenario: Hidden controls are not fields

- **WHEN** the page carries controls the user cannot see (a hidden recaptcha input, a collapsed section)
- **THEN** they are not reported, so the agent neither reads nor writes them

#### Scenario: A widget's options travel with it when readable

- **WHEN** a combobox exposes its options without being opened
- **THEN** `read_form` includes them, so the agent can plan without a round trip
