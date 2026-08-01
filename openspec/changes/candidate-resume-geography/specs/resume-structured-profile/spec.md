## MODIFIED Requirements

### Requirement: The structured résumé is read-only and tied to the current résumé

The stored structured résumé SHALL be read-only — this capability provides no per-field editing. It SHALL always describe the résumé currently stored for the user: a re-upload re-derives it, and it is served only when its stamp matches the current résumé's upload time. A structured résumé whose stamp does not match the current résumé (a newer CV whose extraction has not yet landed, or a persistent extraction outage) MUST be treated as absent rather than served. Deleting the résumé SHALL clear the stored structured résumé.

Persisting the structure SHALL also derive and store the candidate's geography from the structure's location line, in the same statement and under the same stamp, so that a stored geography can never describe a different CV than the structure it was derived from. The derivation is deterministic and performs no I/O; the rules governing what it emits belong to the `candidate-geography` capability. Deleting the résumé SHALL clear the derived geography along with the structure.

#### Scenario: Re-upload re-derives the structure

- **WHEN** a user who already has a structured résumé uploads a new CV
- **THEN** the structured résumé is re-derived from the new CV in the background and, once persisted, replaces the previous one

#### Scenario: A structure from a superseded résumé is not served

- **WHEN** a newer résumé has been uploaded but its structured extraction has not yet completed
- **THEN** the read surface reports no structured résumé rather than the structure derived from the superseded CV

#### Scenario: Deleting the résumé clears the structure

- **WHEN** a signed-in user deletes their stored résumé
- **THEN** the stored structured résumé is cleared along with the résumé pointer

#### Scenario: Persisting the structure persists the derived geography

- **WHEN** a structured résumé is persisted for a user
- **THEN** the geography derived from its location line is stored in the same write, under the same résumé-upload stamp

#### Scenario: A superseded extraction writes neither the structure nor the geography

- **WHEN** a background extraction completes for a résumé that has since been replaced
- **THEN** the write matches no row and neither the structure nor the derived geography is changed

#### Scenario: Deleting the résumé clears the derived geography

- **WHEN** a signed-in user deletes their stored résumé
- **THEN** the derived geography is cleared along with the structure
