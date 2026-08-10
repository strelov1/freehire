## ADDED Requirements

### Requirement: The same tools, whichever client the harness uses

Both clients SHALL offer the same browser primitives under the same names, taking
the same arguments and returning the same outcomes: `read_form`, `fill_simple`,
`combobox.open`, `combobox.options`, `combobox.select`, `combobox.verify`. A loop
written against one client SHALL work against the other.

#### Scenario: A loop moves between clients

- **WHEN** an agent drives a form through the CLI and then through the MCP server
- **THEN** the same sequence of calls produces the same outcomes

#### Scenario: The tool list is documented, not discovered

- **WHEN** a client adds a primitive
- **THEN** it is the one named in this capability, so the two clients cannot
  diverge by each reading the extension's source differently

### Requirement: Reading the form gives the harness what it needs to decide

`read_form` SHALL report the page's questions with their label, type, current
value, whether each is a custom-widget combobox, and the options it offers where
those are readable — including a multi-control question reported as one field.

#### Scenario: A question rendered as many checkboxes

- **WHEN** the form asks one question through a group of checkboxes
- **THEN** the harness sees one question offering those options, not one per
  control

#### Scenario: A widget whose options are not readable yet

- **WHEN** a custom widget renders no options until it is opened
- **THEN** it is reported as a combobox carrying no options, so the harness knows
  to open it rather than assuming it has none

### Requirement: A widget is driven, not written at

A harness SHALL fill a custom widget by opening it, reading the options it offers,
selecting one of them, and verifying what was committed. Clients SHALL NOT offer a
primitive that writes text into a widget.

#### Scenario: The four steps are available separately

- **WHEN** a harness drives a widget
- **THEN** it can open, read, select and verify as distinct calls, deciding
  between them

#### Scenario: Widget state survives between calls

- **WHEN** a harness opens a widget and reads its options in two separate calls
- **THEN** the second call sees the widget the first one opened, because the state
  belongs to the page rather than to the connection

### Requirement: An unverified write is not a fill

A client SHALL report a widget as filled only when verification confirms the
committed value. A mismatch, a widget that would not open, and a commit the widget
did not take SHALL each be reported as their own outcome.

#### Scenario: The widget committed something else

- **WHEN** verification reports a value other than the one selected
- **THEN** the harness is told the write failed, and the field is not counted as
  filled

#### Scenario: A question addressed by a label that matches several widgets

- **WHEN** a label addresses more than one widget on the page
- **THEN** the harness is told the label is ambiguous, and does not retry blindly

### Requirement: Value-level judgement belongs to the harness

The clients SHALL NOT filter a value against the user's profile. The extension
already refuses an option a widget does not offer and confirms what was committed;
whether a value is *justified* is the loop owner's decision, and on this path the
loop owner is the user.

#### Scenario: The harness chooses its own answer

- **WHEN** a harness selects an option the user's profile does not support
- **THEN** the clients pass the choice through, because the user's own agent
  driving the user's own browser is theirs to direct

#### Scenario: The difference is stated

- **WHEN** a user reads the harness documentation
- **THEN** it says that freehire's hosted autofill filters values against the
  profile and a local harness does not, so the difference is not discovered by
  surprise

### Requirement: Nothing here submits a form

No primitive SHALL submit a form, upload a file, or take any other irreversible
action on the page.

#### Scenario: The harness cannot submit

- **WHEN** a harness has filled every field it can
- **THEN** submitting remains the user's action, and no tool offers to do it
