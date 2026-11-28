## ADDED Requirements

### Requirement: Unplaced achievements can be promoted to a project employment

The owner MUST be able to create a `kind=project` employment and attach an existing unplaced atom to it (create employment, then update the atom with that `employment_id`), using the same authenticated employment and atom write APIs the Experience UI uses. After promotion, the atom MUST appear under that project on bank reads and MUST leave the unplaced set. Automated tests MUST cover create-project-then-attach for an unplaced atom.

#### Scenario: Create project and attach unplaced atom

- **WHEN** an owner creates a valid project employment and updates an unplaced atom to that employment id
- **THEN** a subsequent bank read lists the atom under that project and not among unplaced achievements

#### Scenario: Invalid attach is refused

- **WHEN** an owner attempts to attach an atom to an employment they do not own
- **THEN** the update fails without moving the atom
