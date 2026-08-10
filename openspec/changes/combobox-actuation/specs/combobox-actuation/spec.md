## ADDED Requirements

### Requirement: combobox.open primitive

The extension SHALL expose `combobox.open`, which reveals the listbox of the
custom-widget combobox carrying a given label and reports whether it opened. A
widget that does not open SHALL be reported as such rather than treated as open.

#### Scenario: Widget opens

- **WHEN** `combobox.open` is called for a custom-widget combobox
- **THEN** the widget's listbox is displayed and the call reports it open

#### Scenario: Widget refuses to open

- **WHEN** the widget does not respond to the events the extension can dispatch
- **THEN** the call reports it did not open, so the agent stops rather than acting blind

#### Scenario: Label does not address a widget

- **WHEN** `combobox.open` is called with a label that matches no combobox
- **THEN** it reports not-found, as `fill_simple` does for an unknown label

### Requirement: combobox.options primitive

The extension SHALL expose `combobox.options`, returning the option texts the
open widget currently offers. The options are evidence for the agent's choice:
it selects among values the widget actually has, rather than guessing a string.

#### Scenario: Reports the offered options

- **WHEN** `combobox.options` is called on an open widget
- **THEN** it returns the text of each option the widget is offering

#### Scenario: Closed widget yields nothing

- **WHEN** `combobox.options` is called on a widget that is not open
- **THEN** it reports no options rather than a stale or empty-looking list

### Requirement: combobox.select primitive

The extension SHALL expose `combobox.select`, which commits one of the offered
options by acting on that option — not by writing text into the control. It SHALL
select only an option present in the widget's own list.

#### Scenario: Commits an offered option

- **WHEN** `combobox.select` is called with an option the widget offers
- **THEN** that option is committed and the widget closes

#### Scenario: Refuses a value the widget does not offer

- **WHEN** `combobox.select` is called with text matching none of the options
- **THEN** nothing is committed and the call reports the value was not available

### Requirement: combobox.verify primitive

The extension SHALL expose `combobox.verify`, reporting the value the widget now
holds as displayed by the widget itself. A write whose committed value does not
match what was requested SHALL be reported as failed, never as filled.

#### Scenario: Confirms what was committed

- **WHEN** `combobox.verify` runs after a successful select
- **THEN** it reports the widget's committed value

#### Scenario: A mismatched commit is a failure

- **WHEN** the widget committed a value other than the one selected
- **THEN** the mismatch is reported, and the field is not counted as filled

### Requirement: Widget actuation without debugging permissions

The extension SHALL drive widgets with events it can dispatch from an injected
script, and SHALL NOT require the `debugger` permission. A widget that cannot be
driven this way SHALL be reported as not-fillable rather than forced.

#### Scenario: No new permission is requested

- **WHEN** the extension is installed
- **THEN** it asks for no debugging permission, and the browser shows no debugging banner

#### Scenario: An undrivable widget is reported

- **WHEN** a widget does not respond to dispatched events
- **THEN** it is reported as not-fillable, leaving the field for the user
