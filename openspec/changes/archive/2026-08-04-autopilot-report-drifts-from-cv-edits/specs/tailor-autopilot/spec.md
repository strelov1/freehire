## MODIFIED Requirements

### Requirement: A run records a per-requirement report on the tailored CV

The system SHALL persist, on the tailored CV, a report naming every requirement the run considered
and the outcome of each, so the workspace can show what a run achieved after a page reload. An
outcome MUST be one of a fixed vocabulary — closed from the experience bank, closed from what the
candidate said, still open, or not reached — and a value outside that vocabulary MUST be refused with
a message naming the valid ones rather than persisted. The report tool MUST replace the whole report
on each call and MUST return a receipt rather than echoing the report back into the conversation.

An edit that closes a requirement MAY also merge that outcome into the report in the SAME call —
`cv_edit` accepts an optional requirement and an outcome restricted to `closed_bank` or
`closed_candidate`, the only two a `cv_edit` call can produce. The merge replaces the report's
existing entry for that requirement text (matched case- and whitespace-insensitively) if one
exists, or appends a new entry if none does. This does not replace the whole-report tool: it is
a second way to keep one entry current without depending on a later whole-report call for it.

#### Scenario: The report survives a reload

- **WHEN** a run finishes and the workspace is reloaded
- **THEN** the report of that run is read back with the same requirements and outcomes

#### Scenario: An out-of-vocabulary outcome is refused

- **WHEN** the agent reports an outcome outside the fixed vocabulary
- **THEN** the call is refused with a message naming the valid outcomes, and the stored report is unchanged

#### Scenario: A later answer updates the report in place

- **WHEN** the candidate confirms experience for an open requirement after the run and the agent writes it into the CV
- **THEN** the report is replaced with one marking that requirement closed from what the candidate said

#### Scenario: An edit closes a requirement the report already marks open

- **WHEN** `cv_edit` is called with a requirement and `closed_bank` naming a requirement the
  stored report currently marks `open`
- **THEN** that entry's status becomes `closed_bank` and every other entry in the report is
  unchanged

#### Scenario: An edit closes a requirement no report has an entry for yet

- **WHEN** `cv_edit` is called with a requirement and an outcome, and the CV's report holds no
  entry whose text matches
- **THEN** a new entry for that requirement is appended to the report

#### Scenario: An edit naming no requirement leaves the report untouched

- **WHEN** `cv_edit` is called without a requirement
- **THEN** the CV's stored report, if any, is unchanged by that call

#### Scenario: cv_edit cannot report a requirement as open or not reached

- **WHEN** `cv_edit` is called with a requirement and an outcome outside `closed_bank` or
  `closed_candidate`
- **THEN** the call is refused and the stored report is unchanged
