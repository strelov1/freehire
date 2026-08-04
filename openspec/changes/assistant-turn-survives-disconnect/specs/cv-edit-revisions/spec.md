## ADDED Requirements

### Requirement: A batch is judged by what it asks for, not by how it was packaged

The system SHALL accept an edit batch whose operations arrived as a JSON string
holding the array, applying it exactly as it would the array itself. Packaging is
the model's guess about a wire format and carries no meaning about the edit, so
refusing it spends a round of the turn's budget and teaches the model nothing
about the document.

This tolerance SHALL extend to packaging alone. Unknown fields inside an
operation SHALL still be refused, because a field the editor does not know is a
field it would silently drop — which is how an agent once rewrote the wrong
experience entry while believing it had succeeded.

#### Scenario: Operations arrive as a JSON string

- **WHEN** a batch names its operations as a string holding a JSON array
- **THEN** the batch applies exactly as if the array had been sent directly

#### Scenario: An unknown field is still refused

- **WHEN** an operation carries a field the editor does not define
- **THEN** the batch is refused and names the offending field, and the document is unchanged

### Requirement: A batch's removals do not invalidate each other

When a batch removes more than one element of the same list, the system SHALL
apply those removals so that each address means what it meant when the batch was
written. Removing an earlier position first shifts every later one, so a batch
naming two positions of the same list would refuse itself — a failure caused by
the order of application and not by anything the caller asked for.

#### Scenario: Two positions of one list are removed

- **WHEN** a batch removes positions 3 and 4 of a four-element list
- **THEN** both named elements are removed and the batch is not refused

#### Scenario: A genuinely out-of-range address is still refused

- **WHEN** a batch names a position the list never held
- **THEN** the batch is refused and the document is unchanged
