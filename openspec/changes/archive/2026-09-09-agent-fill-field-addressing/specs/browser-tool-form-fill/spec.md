## ADDED Requirements

### Requirement: A fill addresses one control by label and the scope it was read from

The `fill_simple` primitive SHALL accept, on each fill, the `label` naming the question and
the optional `frame` and `form` indices scoping it — the same two indices `read_form`
reports on every field it returns. The extension SHALL read all three off the wire; a scope
a harness sends SHALL NOT be discarded.

`frame` narrows a fill to one of the tab's documents (0 is the top document); `form` narrows
it further to one `<form>` within that document, and SHALL admit **-1**, the index of a
question standing outside any form — the shape Ashby renders its application in. Together
they narrow a fill to one form, so a re-render or a second form carrying the same label
cannot redirect the write.

They do not make `(label, frame, form)` a key: a label repeated inside ONE form — a
multi-entry section asking "Employer" once per job — still resolves to the first of them.
Closing that needs a per-control identity the wire does not carry, and is out of scope here.

A fill naming no `frame` SHALL continue to be offered to every frame, and a fill naming no
`form` SHALL continue to match within the frame's whole document — the scopes narrow a
fill, they are not required of one.

#### Scenario: A scoped fill writes only into the control it names

- **WHEN** a harness sends a fill for label "Email" naming frame 1 and form 0, and the tab's
  top document also carries a question labelled "Email"
- **THEN** the write lands in frame 1's form 0 and the top document is untouched

#### Scenario: The scope a harness sends survives the wire

- **WHEN** the extension receives a `fill_simple` call whose fills carry `frame` and `form`
- **THEN** both indices reach the page's fill logic with the values that were sent

#### Scenario: An unscoped fill is still accepted

- **WHEN** a harness sends a fill carrying only `label` and `value`, and exactly one question
  on the page carries that label
- **THEN** the write lands in that question and the outcome is `filled`

### Requirement: A fill that resolves to more than one control writes nothing

Where a fill names no `form` and more than one question in the frame carries its label, the
system SHALL write nothing for that fill and SHALL report the outcome `ambiguous`. It SHALL
NOT fall back to the first match.

A careers page routinely carries the application form and a job-alert signup, both asking
for an email address. Guessing between them writes the candidate's details into a form they
did not choose, and reports it as success. Refusing returns the choice to the harness, which
holds the `form` index that resolves it.

#### Scenario: Two questions share a label and the fill names no form

- **WHEN** a fill for label "Email" names no `form`, and the frame carries two questions
  labelled "Email"
- **THEN** neither control is written to and the outcome is `ambiguous`

#### Scenario: Naming the form resolves the ambiguity

- **WHEN** the same page receives a fill for label "Email" naming form 1
- **THEN** only form 1's "Email" is written to and the outcome is `filled`

### Requirement: An outcome names the obstacle that stopped the write

The system SHALL report, for every requested fill, an outcome distinguishing why it did not
land, rather than collapsing every miss into "not found". The vocabulary SHALL be:

- `filled` — the value was written.
- `deferred_combobox` — the question is a custom-widget combobox, reported rather than
  written into.
- `no_option` — the control was found, but offers no option matching the value.
- `ambiguous` — the label matched more than one question and the fill named no form.
- `wrong_form` — the label exists in the frame, but not inside the `form` the fill named.
- `not_fillable` — the control was found and is disabled or not visible.
- `not_found` — no question in the frame carries the label at all.

A harness reads these to decide its next step, and the three added members each carry a
different next step: `ambiguous` means re-send naming a form, `wrong_form` means the form
index is wrong, and `not_fillable` means no re-send will help until the page changes. Today
all three arrive as `not_found`, which invites a retry that cannot succeed.

#### Scenario: A label present in another form is not reported as absent

- **WHEN** a fill names form 0 for a label that exists only in form 1
- **THEN** the outcome is `wrong_form`

#### Scenario: A disabled control is not reported as absent

- **WHEN** a fill addresses a question whose only control is disabled or hidden
- **THEN** the outcome is `not_fillable`

#### Scenario: A label nothing on the page carries is reported as absent

- **WHEN** a fill addresses a label no question in the frame carries
- **THEN** the outcome is `not_found`

### Requirement: Folding the frames' answers keeps the most informative one

Because an unscoped fill is offered to every frame, the extension SHALL fold the frames'
outcomes into one answer per label, keeping the most informative. A frame that does not hold
the control answers `not_found`, and that negative SHALL NOT displace an informative answer
from the frame that does.

The order, least to most informative, SHALL be: `not_found`, `not_fillable`, `wrong_form`,
`no_option`, `ambiguous`, `deferred_combobox`, `filled`. `ambiguous` outranks the other
refusals because it is the one a harness can act on by re-sending; ties keep the first frame
that reported the outcome.

#### Scenario: One frame answers and the rest do not hold the control

- **WHEN** a fill is offered to three frames and only the second holds the question
- **THEN** the folded outcome is the second frame's, not `not_found`

#### Scenario: A refusal outranks the frames that never saw the control

- **WHEN** one frame reports `ambiguous` for a label and two others report `not_found`
- **THEN** the folded outcome is `ambiguous`

### Requirement: The Go agent names the scope it read a field from

`autofillagent` SHALL read the `form` index `read_form` reports on each field, alongside the
`frame` it already reads, and SHALL carry both onto every fill it sends. An agent-planned
fill SHALL therefore never be unscoped.

The frame index has been sent since frame scoping was introduced and the form index has not,
so an agent's fills could be narrowed to a document but never to a form within it.

#### Scenario: A planned fill carries the field's own scope

- **WHEN** the agent plans a fill from a field `read_form` reported in frame 1, form 2
- **THEN** the `fill_simple` call carries `frame: 1` and `form: 2` for that fill

#### Scenario: The contract holds across the language boundary

- **WHEN** the JSON body the Go agent sends for a scoped fill is parsed by the extension's
  fill-argument reader
- **THEN** the resulting fill carries the same `frame` and `form` the agent set
