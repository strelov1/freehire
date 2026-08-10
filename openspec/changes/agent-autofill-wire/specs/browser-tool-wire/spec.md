## ADDED Requirements

### Requirement: JWT-authenticated tool channel

The wire SHALL authenticate both ends with the session JWT (the same token the
unified auth issues) and SHALL relay tool calls only between a harness and the
extension belonging to the **same** user. A harness SHALL NOT reach another
user's extension.

#### Scenario: Authenticated harness reaches its own extension

- **WHEN** a harness and an extension, both presenting a valid JWT for user U, are connected to the relay
- **THEN** a tool call from the harness is forwarded to U's extension and its result is returned to the harness

#### Scenario: Cross-user isolation

- **WHEN** a harness for user A sends a tool call while only user B's extension is connected
- **THEN** the relay does not forward the call to B's extension

#### Scenario: Unauthenticated connection rejected

- **WHEN** a connection presents no valid JWT
- **THEN** the relay refuses it

### Requirement: Request/response tool framing

The wire SHALL carry tool calls as `{ id, tool, args }` and results as
`{ id, result }`, correlating each result to its call by `id`, so multiple calls
can be in flight without ambiguity.

#### Scenario: Correlated result

- **WHEN** the harness sends two tool calls with distinct ids
- **THEN** each result is delivered tagged with the id of its originating call

#### Scenario: Executor error surfaces as a result

- **WHEN** the extension fails to execute a tool
- **THEN** an error result tagged with the call id is returned to the harness (the call does not hang silently)

### Requirement: read_form primitive

The extension SHALL expose `read_form`, returning the page's fillable controls
with a stable, human-meaningful **label** for each (from `<label>`,
`aria-labelledby`, `aria-label`, placeholder, or name), across frames.

#### Scenario: Reads labelled fields

- **WHEN** `read_form` runs on a page with a form
- **THEN** it returns each fillable control's label, type, current value, and whether it is a custom-widget combobox

#### Scenario: Reaches iframe'd forms

- **WHEN** the form is inside an iframe
- **THEN** `read_form` includes that frame's fields (tagged by frame)

### Requirement: fill_simple primitive

The extension SHALL expose `fill_simple`, filling standard controls
(text, textarea, checkbox, native `<select>`) addressed by **label**, matching
and writing in one atomic pass so a form re-render cannot misalign the target.
It SHALL dispatch native input/change events so React/Angular forms register the
change, and SHALL report which fields were written.

#### Scenario: Fills standard fields by label

- **WHEN** `fill_simple` is called with `[{label, value}]` for text/checkbox/select fields
- **THEN** each value is written to the control whose label matches, and the write is reported

#### Scenario: No index drift across a re-render

- **WHEN** the form re-renders (changing its control count) between reading and filling
- **THEN** values still land on the correct fields, because addressing is by label, not position

#### Scenario: Custom-widget fields are not mis-filled

- **WHEN** a plan entry targets a custom-widget combobox
- **THEN** `fill_simple` skips it (reports it deferred) rather than writing stale text into it
