## MODIFIED Requirements

### Requirement: A run is revertable in one move

The system SHALL group every revision a run commits under that run's own identifier, and SHALL
offer an owner-scoped revert that undoes those revisions in reverse order. Reverting a whole run
MUST clear the run report, because a report describing edits that no longer exist misdescribes
the CV. A CV with no run to revert MUST report that there is nothing to revert rather than
altering the document.

The pre-run document snapshot is retired. Grouping by run rather than snapshotting removes the
edge two concurrent runs used to create — each took its own snapshot, and reverting returned the
document to the middle of the other run.

Undoing a single revision of a run MUST NOT clear the report: the report is about requirements,
not edits, and remains largely true while one of its edits is reversed. Only reverting the whole
run clears it.

#### Scenario: Reverting restores the pre-run document

- **WHEN** the owner reverts after a run
- **THEN** every revision the run committed is undone in reverse order, and the tailored CV holds the document it had before the run started

#### Scenario: Reverting clears the run's traces

- **WHEN** a whole-run revert completes
- **THEN** the report is cleared and the workspace offers to start a run again

#### Scenario: Reverting without a run is refused

- **WHEN** a revert is requested for a CV no run has edited
- **THEN** the request is refused and the document is unchanged

#### Scenario: Two concurrent runs revert independently

- **WHEN** two runs edit the same CV and the first is reverted
- **THEN** only the first run's edits are undone and the second run's edits remain

#### Scenario: Undoing one edit of a run keeps the report

- **WHEN** the owner undoes a single revision belonging to a run
- **THEN** that edit is reversed and the run report is still shown
